package scopevisio

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshTokenUsesScopevisioConfigFields(t *testing.T) {
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
		BaseURL:      server.URL,
		Customer:     "1234567",
		RefreshToken: "refresh-0",
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

func TestJSONRequestUsesScopevisioConfig(t *testing.T) {
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
			"actions": map[string]any{"add-tag": []string{"scopevisio-test"}},
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
