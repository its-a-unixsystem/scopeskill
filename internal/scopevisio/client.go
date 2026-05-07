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

const DefaultBaseURL = "https://appload.scopevisio.com"

// Future two-factor Auth login support should submit the one-time password as totpResponse.
const totpResponseFormField = "totpResponse"

type Config struct {
	ConfigPath   string
	BaseURL      string
	Customer     string
	RefreshToken string
	AccessToken  string
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
	if c.Config.AccessToken != "" {
		return Token{
			TokenType:   "Bearer",
			AccessToken: c.Config.AccessToken,
			ObtainedAt:  time.Now().Unix(),
		}, nil
	}
	if c.Config.RefreshToken == "" {
		return Token{}, errors.New("missing REST refresh token; run scopevisio auth login or set REST_REFRESH_TOKEN in the Scopevisio config")
	}
	return c.RefreshToken(c.Config.RefreshToken)
}

func (c *Client) RefreshToken(refreshToken string) (Token, error) {
	if c.Config.Customer == "" {
		return Token{}, errors.New("missing CUSTOMER in Scopevisio config; refresh-token exchange requires CUSTOMER with REST_REFRESH_TOKEN")
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
	return token, nil
}

func (c *Client) authorize(req *http.Request) error {
	token, err := c.GetToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
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
	baseURL := strings.TrimRight(c.Config.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/rest") {
		baseURL += "/rest"
	}
	return withQuery(baseURL+"/"+strings.TrimLeft(path, "/"), query)
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
