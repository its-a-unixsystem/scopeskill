package scopevisio

import (
	"bytes"
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

const DefaultBaseURL = "https://appload.scopevisio.com/rest"

type Config struct {
	BaseURL        string
	Customer       string
	Organisation   string
	OrganisationID string
	Username       string
	Password       string
	ClientID       string
	ClientSecret   string
	RefreshToken   string
	AccessToken    string
	TokenCache     string
	AuthHeader     string
}

type Token struct {
	TokenType    string         `json:"token_type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	ExpiresIn    int64          `json:"expires_in,omitempty"`
	ObtainedAt   int64          `json:"obtained_at,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
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

func ConfigFromEnv() Config {
	cfg := Config{
		BaseURL:        getenv("SCOPEVISIO_BASE_URL", DefaultBaseURL),
		Customer:       os.Getenv("SCOPEVISIO_CUSTOMER"),
		Organisation:   os.Getenv("SCOPEVISIO_ORGANISATION"),
		OrganisationID: os.Getenv("SCOPEVISIO_ORGANISATION_ID"),
		Username:       os.Getenv("SCOPEVISIO_USERNAME"),
		Password:       os.Getenv("SCOPEVISIO_PASSWORD"),
		ClientID:       os.Getenv("SCOPEVISIO_CLIENT_ID"),
		ClientSecret:   os.Getenv("SCOPEVISIO_CLIENT_SECRET"),
		RefreshToken:   os.Getenv("SCOPEVISIO_REFRESH_TOKEN"),
		AccessToken:    os.Getenv("SCOPEVISIO_ACCESS_TOKEN"),
		TokenCache:     os.Getenv("SCOPEVISIO_TOKEN_CACHE"),
		AuthHeader:     getenv("SCOPEVISIO_AUTH_HEADER", "Authorization"),
	}
	if cfg.TokenCache == "" {
		cfg.TokenCache = defaultTokenCache()
	}
	return cfg
}

func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = "Authorization"
	}
	if cfg.TokenCache == "" {
		cfg.TokenCache = defaultTokenCache()
	}
	return &Client{
		Config:     cfg,
		HTTPClient: http.DefaultClient,
	}
}

func (c *Client) GetToken(totp string) (Token, error) {
	if c.Config.AccessToken != "" {
		return Token{
			TokenType:    "Bearer",
			AccessToken:  c.Config.AccessToken,
			RefreshToken: c.Config.RefreshToken,
			ObtainedAt:   time.Now().Unix(),
		}, nil
	}

	cached, cacheErr := c.loadToken()
	if cacheErr == nil && cached.AccessToken != "" && !cached.Expired() {
		return cached, nil
	}

	refreshToken := c.Config.RefreshToken
	if refreshToken == "" {
		refreshToken = cached.RefreshToken
	}
	if refreshToken != "" {
		return c.RefreshToken(refreshToken)
	}

	return c.PasswordToken(totp)
}

func (c *Client) PasswordToken(totp string) (Token, error) {
	missing := missingEnv(map[string]string{
		"SCOPEVISIO_CUSTOMER": c.Config.Customer,
		"SCOPEVISIO_USERNAME": c.Config.Username,
		"SCOPEVISIO_PASSWORD": c.Config.Password,
	})
	if len(missing) > 0 {
		return Token{}, fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("customer", c.Config.Customer)
	form.Set("username", c.Config.Username)
	form.Set("password", c.Config.Password)
	if c.Config.Organisation != "" {
		form.Set("organisation", c.Config.Organisation)
	}
	if c.Config.OrganisationID != "" {
		form.Set("organisation_id", c.Config.OrganisationID)
	}
	if totp != "" {
		form.Set("totpResponse", totp)
	}
	return c.requestToken(form)
}

func (c *Client) RefreshToken(refreshToken string) (Token, error) {
	if c.Config.Customer == "" {
		return Token{}, errors.New("missing environment variable: SCOPEVISIO_CUSTOMER")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("customer", c.Config.Customer)
	form.Set("refresh_token", refreshToken)
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
	if err := c.authorize(req); err != nil {
		return nil, err
	}

	raw, err := c.do(req)
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

func (c *Client) Bytes(method string, path string, body io.Reader, headers map[string]string, query map[string]string) ([]byte, error) {
	req, err := http.NewRequest(method, c.url(path, query), body)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if err := c.authorize(req); err != nil {
		return nil, err
	}
	return c.do(req)
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
	if t.ExpiresIn == 0 {
		return false
	}
	return time.Now().Unix() >= t.ObtainedAt+t.ExpiresIn-60
}

func (c *Client) requestToken(form url.Values) (Token, error) {
	if c.Config.ClientID != "" {
		form.Set("client_id", c.Config.ClientID)
	}
	if c.Config.ClientSecret != "" {
		form.Set("client_secret", c.Config.ClientSecret)
	}

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
	token := Token{
		TokenType:    stringValue(payload["token_type"], "Bearer"),
		AccessToken:  stringValue(payload["access_token"], ""),
		RefreshToken: stringValue(payload["refresh_token"], ""),
		ExpiresIn:    int64Value(payload["expires_in"]),
		ObtainedAt:   time.Now().Unix(),
		Payload:      payload,
	}
	if token.AccessToken == "" {
		return Token{}, errors.New("token response did not include access_token")
	}
	return token, c.saveToken(token)
}

func (c *Client) authorize(req *http.Request) error {
	token, err := c.GetToken("")
	if err != nil {
		return err
	}
	req.Header.Set(c.Config.AuthHeader, token.TokenType+" "+token.AccessToken)
	return nil
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
	return withQuery(strings.TrimRight(c.Config.BaseURL, "/")+"/"+strings.TrimLeft(path, "/"), query)
}

func (c *Client) loadToken() (Token, error) {
	raw, err := os.ReadFile(c.Config.TokenCache)
	if err != nil {
		return Token{}, err
	}
	var token Token
	err = json.Unmarshal(raw, &token)
	return token, err
}

func (c *Client) saveToken(token Token) error {
	if err := os.MkdirAll(filepath.Dir(c.Config.TokenCache), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.Config.TokenCache, raw, 0o600)
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

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func defaultTokenCache() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return ".scopevisio-token.json"
		}
		return filepath.Join(home, ".config", "scopeskill", "token.json")
	}
	return filepath.Join(configDir, "scopeskill", "token.json")
}

func missingEnv(values map[string]string) []string {
	var missing []string
	for key, value := range values {
		if value == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func stringValue(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	return fmt.Sprint(value)
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
