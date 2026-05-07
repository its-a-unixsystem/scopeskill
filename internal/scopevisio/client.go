package scopevisio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://appload.scopevisio.com"
const accessTokenCacheSafetyMargin = 30 * time.Second

// Future two-factor Auth login support should submit the one-time password as totpResponse.
const totpResponseFormField = "totpResponse"

type Config struct {
	ConfigPath       string
	BaseURL          string
	Customer         string
	RefreshToken     string
	AccessToken      string
	AccessTokenCache string
}

type InitialCredentials struct {
	Customer       string
	Username       string
	Password       string
	OrganisationID string
}

type Token struct {
	TokenType    string         `json:"token_type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	ExpiresIn    int64          `json:"expires_in,omitempty"`
	ExpiresAt    int64          `json:"expires_at,omitempty"`
	ObtainedAt   int64          `json:"obtained_at,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type accessTokenCache struct {
	TokenType   string `json:"token_type"`
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
}

type Client struct {
	Config     Config
	HTTPClient *http.Client
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("scopevisio returned HTTP %d: %s", e.StatusCode, e.Body)
}

type AuthLoginRequiredError struct {
	Err error
}

func (e AuthLoginRequiredError) Error() string {
	return fmt.Sprintf("%v; run scopevisio auth login", e.Err)
}

type TransientRefreshError struct {
	Err error
}

func (e TransientRefreshError) Error() string {
	return fmt.Sprintf("transient Scopevisio refresh-token exchange failure; try again later: %v", e.Err)
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return &Client{
		Config:     cfg,
		HTTPClient: http.DefaultClient,
	}
}

func (c *Client) GetToken() (Token, error) {
	token, _, err := c.getToken()
	return token, err
}

func (c *Client) getToken() (Token, bool, error) {
	if c.Config.AccessToken != "" {
		return Token{
			TokenType:   "Bearer",
			AccessToken: c.Config.AccessToken,
			ObtainedAt:  time.Now().Unix(),
		}, false, nil
	}
	if c.Config.RefreshToken == "" {
		return Token{}, false, errors.New("missing REST refresh token; run scopevisio auth login or set REST_REFRESH_TOKEN in the Scopevisio config")
	}
	if token, err := c.loadAccessTokenCache(); err == nil && !token.Expired() {
		return token, false, nil
	}
	token, err := c.RefreshToken(c.Config.RefreshToken)
	if err != nil {
		return Token{}, false, err
	}
	if err := c.saveAccessTokenCache(token); err != nil {
		return Token{}, false, err
	}
	return token, true, nil
}

func (c *Client) RefreshToken(refreshToken string) (Token, error) {
	if c.Config.Customer == "" {
		return Token{}, errors.New("missing CUSTOMER in Scopevisio config; refresh-token exchange requires CUSTOMER with REST_REFRESH_TOKEN")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("customer", c.Config.Customer)
	form.Set("refresh_token", refreshToken)
	token, err := c.requestToken(form)
	if err == nil {
		return token, nil
	}
	var apiErr APIError
	switch {
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden):
		_ = c.deleteAccessTokenCache()
		return Token{}, AuthLoginRequiredError{Err: err}
	case errors.As(err, &apiErr) && apiErr.StatusCode >= 500:
		return Token{}, TransientRefreshError{Err: err}
	case !errors.As(err, &apiErr):
		return Token{}, TransientRefreshError{Err: err}
	default:
		return Token{}, err
	}
}

func (c *Client) Login(credentials InitialCredentials) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("customer", credentials.Customer)
	form.Set("username", credentials.Username)
	form.Set("password", credentials.Password)
	if credentials.OrganisationID != "" {
		form.Set("organisation_id", credentials.OrganisationID)
	}
	return c.requestToken(form)
}

func (c *Client) JSON(method string, path string, body any, query map[string]string) (any, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, c.url(path, query), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	freshToken, err := c.authorize(req)
	if err != nil {
		return nil, err
	}

	raw, err := c.do(req)
	if err != nil {
		if err := c.handleFreshTokenAPIError(err, freshToken); err != nil {
			return nil, err
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) Bytes(method string, path string, body io.Reader, headers map[string]string, query map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, c.url(path, query), body)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	freshToken, err := c.authorize(req)
	if err != nil {
		return nil, err
	}
	raw, err := c.do(req)
	if err != nil {
		if err := c.handleFreshTokenAPIError(err, freshToken); err != nil {
			return nil, err
		}
		return nil, err
	}
	return raw, nil
}

func (c *Client) Download(path string, out string, query map[string]string) error {
	raw, err := c.Bytes(http.MethodGet, path, nil, map[string]string{"Accept": "*/*"}, query)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, raw, 0o644)
}

func (c *Client) UploadTeamworkDocument(filePath string, metadata map[string]any) (any, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	payload := ensureMetadata(metadata)
	document := payload["metadata"].(map[string]any)["document"].(map[string]any)
	if _, ok := document["filename"]; !ok {
		document["filename"] = filepath.Base(filePath)
	}
	if _, ok := document["size"]; !ok {
		document["size"] = info.Size()
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metaPart, err := writer.CreatePart(textprotoHeader(map[string]string{
		"Content-Disposition": `form-data; name="metadata"`,
		"Content-Type":        "application/json",
	}))
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(metaPart).Encode(payload); err != nil {
		return nil, err
	}

	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	docPart, err := writer.CreatePart(textprotoHeader(map[string]string{
		"Content-Disposition": fmt.Sprintf(`form-data; name="document"; filename="%s"`, filepath.Base(filePath)),
		"Content-Type":        mimeType,
	}))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(docPart, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	raw, err := c.Bytes(http.MethodPost, "/teamworkbridge/documents", &body, map[string]string{
		"Accept":       "application/json",
		"Content-Type": writer.FormDataContentType(),
	}, nil)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var result any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (t Token) Expired() bool {
	if t.ExpiresAt != 0 {
		return time.Now().Add(accessTokenCacheSafetyMargin).Unix() >= t.ExpiresAt
	}
	if t.ExpiresIn == 0 {
		return false
	}
	return time.Now().Unix() >= t.ObtainedAt+t.ExpiresIn-60
}

func (c *Client) requestToken(form url.Values) (Token, error) {
	req, err := http.NewRequest(http.MethodPost, c.url("/token", nil), strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	raw, err := c.do(req)
	if err != nil {
		return Token{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Token{}, err
	}
	now := time.Now()
	token := Token{
		TokenType:    stringValue(payload["token_type"], "Bearer"),
		AccessToken:  stringValue(payload["access_token"], ""),
		RefreshToken: stringValue(payload["refresh_token"], ""),
		ExpiresIn:    int64Value(payload["expires_in"]),
		ObtainedAt:   now.Unix(),
		Payload:      payload,
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second).Unix()
	}
	if token.AccessToken == "" {
		return Token{}, errors.New("token response did not include access_token")
	}
	return token, nil
}

func (c *Client) authorize(req *http.Request) (bool, error) {
	token, freshToken, err := c.getToken()
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
	return freshToken, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	response, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, APIError{StatusCode: response.StatusCode, Body: string(raw)}
	}
	return raw, nil
}

func (c *Client) url(path string, query map[string]string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return withQuery(path, query)
	}
	baseURL := strings.TrimRight(c.Config.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/rest") {
		baseURL += "/rest"
	}
	return withQuery(baseURL+"/"+strings.TrimLeft(path, "/"), query)
}

func (c *Client) handleFreshTokenAPIError(err error, freshToken bool) error {
	var apiErr APIError
	if !freshToken || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		return nil
	}
	_ = c.deleteAccessTokenCache()
	return AuthLoginRequiredError{Err: err}
}

func (c *Client) loadAccessTokenCache() (Token, error) {
	raw, err := os.ReadFile(c.accessTokenCachePath())
	if err != nil {
		return Token{}, err
	}
	var cached accessTokenCache
	if err := json.Unmarshal(raw, &cached); err != nil {
		return Token{}, err
	}
	if cached.AccessToken == "" || cached.ExpiresAt == 0 {
		return Token{}, errors.New("Access token cache is incomplete")
	}
	return Token{
		TokenType:   valueOrDefault(cached.TokenType, "Bearer"),
		AccessToken: cached.AccessToken,
		ExpiresAt:   cached.ExpiresAt,
	}, nil
}

func (c *Client) saveAccessTokenCache(token Token) error {
	path := c.accessTokenCachePath()
	if err := ensurePrivateConfigDir(filepath.Dir(path)); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(accessTokenCache{
		TokenType:   token.TokenType,
		AccessToken: token.AccessToken,
		ExpiresAt:   token.ExpiresAt,
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (c *Client) deleteAccessTokenCache() error {
	err := os.Remove(c.accessTokenCachePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (c *Client) accessTokenCachePath() string {
	if c.Config.AccessTokenCache != "" {
		return c.Config.AccessTokenCache
	}
	return DefaultAccessTokenCachePath(c.Config.RefreshToken)
}

func ensureMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	meta, ok := metadata["metadata"].(map[string]any)
	if !ok {
		meta = map[string]any{}
		metadata["metadata"] = meta
	}
	document, ok := meta["document"].(map[string]any)
	if !ok {
		document = map[string]any{}
		meta["document"] = document
	}
	return metadata
}

func withQuery(raw string, query map[string]string) string {
	if len(query) == 0 {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func stringValue(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprint(value)
}

func DefaultAccessTokenCachePath(refreshToken string) string {
	cacheDir, err := os.UserCacheDir()
	if err == nil {
		return filepath.Join(cacheDir, "scopeskill", "access-token-"+refreshTokenFingerprint(refreshToken)+".json")
	}
	homeDir, homeErr := os.UserHomeDir()
	if homeErr == nil {
		return filepath.Join(homeDir, ".cache", "scopeskill", "access-token-"+refreshTokenFingerprint(refreshToken)+".json")
	}
	return filepath.Join(".cache", "scopeskill", "access-token-"+refreshTokenFingerprint(refreshToken)+".json")
}

func refreshTokenFingerprint(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(sum[:])[:16]
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(v, 10, 64)
		return parsed
	default:
		return 0
	}
}

func textprotoHeader(values map[string]string) textproto.MIMEHeader {
	header := textproto.MIMEHeader{}
	for key, value := range values {
		header[key] = []string{value}
	}
	return header
}
