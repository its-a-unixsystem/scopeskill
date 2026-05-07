package scopeskill

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRefreshTokenUsesConfigFileFields(t *testing.T) {
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form = r.PostForm
		writeJSON(w, map[string]any{
			"token_type":       "Bearer",
			"access_token":     "access-1",
			"refresh_token":    "refresh-1",
			"expires_in":       3600,
			"uid":              "user_1",
			"organisationId":   7,
			"organisationName": "Example GmbH",
		})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:  server.URL,
		Customer: "1234567",
	})
	token, err := client.RefreshToken("refresh-0")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access-1" {
		t.Fatalf("access token = %q", token.AccessToken)
	}
	if form.Get("grant_type") != "refresh_token" {
		t.Fatalf("grant_type = %q", form.Get("grant_type"))
	}
	if form.Get("customer") != "1234567" {
		t.Fatalf("customer = %q", form.Get("customer"))
	}
	if form.Get("refresh_token") != "refresh-0" {
		t.Fatalf("refresh_token = %q", form.Get("refresh_token"))
	}
}

func TestLoginUsesInitialCredentials(t *testing.T) {
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form = r.PostForm
		writeJSON(w, map[string]any{
			"token_type":    "Bearer",
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	token, err := client.Login(InitialCredentials{
		Customer:       "1234567",
		Username:       "tech@example.com",
		Password:       "secret-password",
		OrganisationID: "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token = %q", token.RefreshToken)
	}
	for key, want := range map[string]string{
		"grant_type":      "password",
		"customer":        "1234567",
		"username":        "tech@example.com",
		"password":        "secret-password",
		"organisation_id": "42",
	} {
		if got := form.Get(key); got != want {
			t.Fatalf("%s = %q", key, got)
		}
	}
}

func TestLoginOmitsEmptyOrganisationID(t *testing.T) {
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		form = r.PostForm
		writeJSON(w, map[string]any{
			"token_type":    "Bearer",
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	token, err := client.Login(InitialCredentials{
		Customer: "1234567",
		Username: "tech@example.com",
		Password: "secret-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.RefreshToken != "refresh-1" {
		t.Fatalf("refresh token = %q", token.RefreshToken)
	}
	if form.Has("organisation_id") {
		t.Fatalf("organisation_id = %q", form.Get("organisation_id"))
	}
}

func TestJSONRequestAddsBearerToken(t *testing.T) {
	var auth string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/token" {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("grant_type") != "refresh_token" {
				t.Fatalf("grant_type = %q", r.PostForm.Get("grant_type"))
			}
			writeJSON(w, map[string]any{
				"token_type":    "Bearer",
				"access_token":  "access-1",
				"refresh_token": "refresh-1",
				"expires_in":    3600,
			})
			return
		}
		if r.URL.Path != "/rest/contacts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-0",
		AccessTokenCache: filepath.Join(t.TempDir(), "access-token.json"),
	})
	result, err := client.JSON("POST", "/contacts", map[string]any{"page": 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer access-1" {
		t.Fatalf("Authorization = %q", auth)
	}
	if requestBody["page"].(float64) != 0 {
		t.Fatalf("page = %v", requestBody["page"])
	}
	if result.(map[string]any)["ok"] != true {
		t.Fatalf("result = %#v", result)
	}
}

func TestJSONRequestUsesConfigFile(t *testing.T) {
	var auth string
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			tokenForm = r.PostForm
			writeJSON(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-config",
				"expires_in":   3600,
			})
		case "/rest/myaccount":
			auth = r.Header.Get("Authorization")
			writeJSON(w, map[string]any{"account": "ok"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("CUSTOMER=1234567\nREST_REFRESH_TOKEN=config-refresh\nBASE_URL="+server.URL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	t.Setenv(EnvRestRefreshToken, "")
	t.Setenv(EnvBaseURL, "")
	config, err := LoadClientConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(config)
	result, err := client.JSON("GET", "/myaccount", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tokenForm.Get("refresh_token") != "config-refresh" {
		t.Fatalf("refresh_token = %q", tokenForm.Get("refresh_token"))
	}
	if auth != "Bearer access-from-config" {
		t.Fatalf("Authorization = %q", auth)
	}
	if result.(map[string]any)["account"] != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAccessTokenCacheMissAndHit(t *testing.T) {
	var refreshCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			refreshCount++
			writeJSON(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/myaccount":
			if got := r.Header.Get("Authorization"); got != "Bearer access-from-refresh" {
				t.Fatalf("Authorization = %q", got)
			}
			writeJSON(w, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "cache", "access-token.json")
	client := NewClient(Config{
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	if _, err := client.JSON("GET", "/myaccount", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.JSON("GET", "/myaccount", nil, nil); err != nil {
		t.Fatal(err)
	}
	if refreshCount != 1 {
		t.Fatalf("refresh count = %d", refreshCount)
	}
	assertPrivateMode(t, filepath.Dir(cachePath), 0o700)
	assertPrivateMode(t, cachePath, 0o600)
}

func TestAccessTokenCacheExpiredMiss(t *testing.T) {
	var refreshCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/token" {
			refreshCount++
			writeJSON(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "fresh-access",
				"expires_in":   3600,
			})
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "access-token.json")
	writeAccessTokenCache(t, cachePath, "expired-access", time.Now().Add(-time.Hour).Unix())
	client := NewClient(Config{
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	if _, err := client.JSON("GET", "/myaccount", nil, nil); err != nil {
		t.Fatal(err)
	}
	if refreshCount != 1 {
		t.Fatalf("refresh count = %d", refreshCount)
	}
}

func TestDefaultAccessTokenCachePathUsesRefreshTokenFingerprint(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	got := DefaultAccessTokenCachePath("refresh-token-a")
	want := filepath.Join(cacheDir, "scopeskill", "access-token-"+refreshTokenFingerprint("refresh-token-a")+".json")
	if got != want {
		t.Fatalf("cache path = %q", got)
	}
	if DefaultAccessTokenCachePath("refresh-token-a") == DefaultAccessTokenCachePath("refresh-token-b") {
		t.Fatal("different REST refresh tokens used the same Access token cache path")
	}
}

func TestDifferentRefreshTokensUseDifferentDefaultCacheFiles(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	var refreshTokens []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			refreshToken := r.PostForm.Get("refresh_token")
			refreshTokens = append(refreshTokens, refreshToken)
			writeJSON(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-for-" + refreshToken,
				"expires_in":   3600,
			})
		case "/rest/myaccount":
			writeJSON(w, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	firstClient := NewClient(Config{
		BaseURL:      server.URL,
		Customer:     "1234567",
		RefreshToken: "refresh-token-a",
	})
	if _, err := firstClient.JSON("GET", "/myaccount", nil, nil); err != nil {
		t.Fatal(err)
	}
	firstCachePath := DefaultAccessTokenCachePath("refresh-token-a")
	firstCacheRaw, err := os.ReadFile(firstCachePath)
	if err != nil {
		t.Fatal(err)
	}

	secondClient := NewClient(Config{
		BaseURL:      server.URL,
		Customer:     "1234567",
		RefreshToken: "refresh-token-b",
	})
	if _, err := secondClient.JSON("GET", "/myaccount", nil, nil); err != nil {
		t.Fatal(err)
	}
	secondCachePath := DefaultAccessTokenCachePath("refresh-token-b")
	if secondCachePath == firstCachePath {
		t.Fatal("default cache paths matched")
	}
	if _, err := os.Stat(secondCachePath); err != nil {
		t.Fatal(err)
	}
	assertFileBytes(t, firstCachePath, firstCacheRaw)
	if strings.Join(refreshTokens, ",") != "refresh-token-a,refresh-token-b" {
		t.Fatalf("refresh tokens = %#v", refreshTokens)
	}
}

func TestAccessTokenCacheOverridePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.json")
	t.Setenv(EnvAccessTokenCache, path)

	config, err := LoadClientConfig(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if config.AccessTokenCache != path {
		t.Fatalf("AccessTokenCache = %q", config.AccessTokenCache)
	}
}

func TestRefreshTokenUnauthorizedDeletesCacheAndKeepsConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	configRaw := []byte("CUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(dir, "access-token.json")
	writeAccessTokenCache(t, cachePath, "old-access", time.Now().Add(time.Hour).Unix())
	client := NewClient(Config{
		ConfigPath:       configPath,
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	_, err := client.RefreshToken("refresh-token")
	if err == nil {
		t.Fatal("expected auth login error")
	}
	var authErr AuthLoginRequiredError
	if !errors.As(err, &authErr) || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("error = %T %v", err, err)
	}
	if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
		t.Fatalf("cache stat error = %v", statErr)
	}
	assertFileBytes(t, configPath, configRaw)
}

func TestRefreshTokenServerErrorLeavesCacheAndConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	configRaw := []byte("CUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(dir, "access-token.json")
	cacheRaw := writeAccessTokenCache(t, cachePath, "old-access", time.Now().Add(time.Hour).Unix())
	client := NewClient(Config{
		ConfigPath:       configPath,
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	_, err := client.RefreshToken("refresh-token")
	if err == nil {
		t.Fatal("expected transient refresh error")
	}
	var transientErr TransientRefreshError
	if !errors.As(err, &transientErr) || !strings.Contains(err.Error(), "transient") {
		t.Fatalf("error = %T %v", err, err)
	}
	assertFileBytes(t, cachePath, cacheRaw)
	assertFileBytes(t, configPath, configRaw)
}

func TestRefreshTokenNetworkErrorLeavesCacheAndConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	configRaw := []byte("CUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(dir, "access-token.json")
	cacheRaw := writeAccessTokenCache(t, cachePath, "old-access", time.Now().Add(time.Hour).Unix())
	client := NewClient(Config{
		ConfigPath:       configPath,
		BaseURL:          baseURL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	_, err := client.RefreshToken("refresh-token")
	if err == nil {
		t.Fatal("expected transient refresh error")
	}
	var transientErr TransientRefreshError
	if !errors.As(err, &transientErr) {
		t.Fatalf("error = %T %v", err, err)
	}
	assertFileBytes(t, cachePath, cacheRaw)
	assertFileBytes(t, configPath, configRaw)
}

func TestFreshAccessTokenUnauthorizedDeletesCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSON(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "fresh-access",
				"expires_in":   3600,
			})
		case "/rest/myaccount":
			http.Error(w, "access denied", http.StatusUnauthorized)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cachePath := filepath.Join(t.TempDir(), "access-token.json")
	client := NewClient(Config{
		BaseURL:          server.URL,
		Customer:         "1234567",
		RefreshToken:     "refresh-token",
		AccessTokenCache: cachePath,
	})
	_, err := client.JSON("GET", "/myaccount", nil, nil)
	if err == nil {
		t.Fatal("expected auth login error")
	}
	var authErr AuthLoginRequiredError
	if !errors.As(err, &authErr) || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("error = %T %v", err, err)
	}
	if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
		t.Fatalf("cache stat error = %v", statErr)
	}
}

func TestDownloadWritesBinaryResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/teamworkbridge/document/doc-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-1" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte("document-bytes"))
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "document.bin")
	client := NewClient(Config{
		BaseURL:     server.URL,
		AccessToken: "access-1",
	})
	if err := client.Download("/teamworkbridge/document/doc-1", out, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "document-bytes" {
		t.Fatalf("downloaded = %q", string(raw))
	}
}

func TestTeamworkUploadUsesMultipartMetadataAndDocumentParts(t *testing.T) {
	var contentType string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/teamworkbridge/documents" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		contentType = r.Header.Get("Content-Type")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
		writeJSON(w, map[string]any{"id": "doc-1"})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "invoice.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient(Config{
		BaseURL:     server.URL,
		AccessToken: "access-1",
	})
	result, err := client.UploadTeamworkDocument(path, map[string]any{
		"metadata": map[string]any{
			"actions": map[string]any{"add-tag": []string{"scopeskill-test"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["id"] != "doc-1" {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(contentType, "multipart/form-data") {
		t.Fatalf("content type = %q", contentType)
	}
	for _, want := range []string{`name="metadata"`, `name="document"; filename="invoice.txt"`, `"filename":"invoice.txt"`, `"size":5`} {
		if !strings.Contains(body, want) {
			t.Fatalf("multipart body missing %q in %s", want, body)
		}
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeAccessTokenCache(t *testing.T, path string, accessToken string, expiresAt int64) []byte {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(accessTokenCache{
		TokenType:   "Bearer",
		AccessToken: accessToken,
		ExpiresAt:   expiresAt,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s = %s, want %s", path, string(got), string(want))
	}
}

func assertPrivateMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o", path, got)
	}
}
