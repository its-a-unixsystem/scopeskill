package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

// loginProbeServer responds to /rest/token with the canned grant, and to
// /rest/impersonalaccounts with a hit for any number listed in `hits`.
func loginProbeServer(t *testing.T, hits map[string]bool, queried *[]string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":    "Bearer",
				"access_token":  "access-from-login",
				"refresh_token": "refresh-from-login",
				"expires_in":    3600,
			})
		case "/rest/impersonalaccounts":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			search := body["search"].([]any)
			number := search[0].(map[string]any)["value"].(string)
			if queried != nil {
				*queried = append(*queried, number)
			}
			if hits[number] {
				writeJSONForCLI(w, []any{map[string]any{"id": 1, "number": number}})
				return
			}
			writeJSONForCLI(w, []any{})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func loginProbeConfig(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("BASE_URL="+baseURL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return path
}

func TestAuthLoginRunsSKRProbeAndWritesSKR04(t *testing.T) {
	queried := []string{}
	server := loginProbeServer(t, map[string]bool{"4400": true}, &queried)
	path := loginProbeConfig(t, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n", true)

	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "SKR=skr04") {
		t.Fatalf("config = %s", raw)
	}
	if len(queried) != 1 || queried[0] != "4400" {
		t.Fatalf("queried = %v", queried)
	}
}

func TestAuthLoginRunsSKRProbeAndWritesSKR03(t *testing.T) {
	server := loginProbeServer(t, map[string]bool{"8400": true}, nil)
	path := loginProbeConfig(t, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n", true)

	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "SKR=skr03") {
		t.Fatalf("config = %s", raw)
	}
}

func TestAuthLoginPromptsWhenNeitherSKRMatches(t *testing.T) {
	server := loginProbeServer(t, map[string]bool{}, nil)
	path := loginProbeConfig(t, server.URL)
	// initial credentials (4 lines) + SKR prompt answer "1" → SKR03
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n1\n", true)

	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "SKR=skr03") {
		t.Fatalf("config = %s", raw)
	}
}

func TestAuthLoginSkipsSKRWhenUserChoosesSkip(t *testing.T) {
	server := loginProbeServer(t, map[string]bool{}, nil)
	path := loginProbeConfig(t, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n3\n", true)

	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "SKR=skr") {
		t.Fatalf("expected no SKR= line, got: %s", text)
	}
}

func TestAuthLoginSKRFlagBypassesProbe(t *testing.T) {
	queried := []string{}
	server := loginProbeServer(t, map[string]bool{}, &queried)
	path := loginProbeConfig(t, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n", true)

	if err := run([]string{"--config", path, "auth", "login", "--skr=skr04"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "SKR=skr04") {
		t.Fatalf("config = %s", raw)
	}
	if len(queried) != 0 {
		t.Fatalf("--skr should bypass the API; queried = %v", queried)
	}
}

func TestAuthLoginReprobeOverwritesSKR(t *testing.T) {
	queried := []string{}
	server := loginProbeServer(t, map[string]bool{"4400": true}, &queried)
	path := filepath.Join(t.TempDir(), "config")
	old := "BASE_URL=" + server.URL + "\nCUSTOMER=old\nREST_REFRESH_TOKEN=old-token\nSKR=skr03\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n", true)

	if err := run([]string{"--config", path, "auth", "login", "--force"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "SKR=skr04") {
		t.Fatalf("expected re-probe to overwrite SKR=skr04: %s", text)
	}
	if strings.Contains(text, "SKR=skr03") {
		t.Fatalf("stale SKR=skr03 still present: %s", text)
	}
}

func TestAuthLoginRejectsInvalidSKRFlag(t *testing.T) {
	server := loginProbeServer(t, nil, nil)
	path := loginProbeConfig(t, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\n\n", true)

	err := run([]string{"--config", path, "auth", "login", "--skr=skr99"})
	if err == nil || !strings.Contains(err.Error(), "invalid --skr") {
		t.Fatalf("err = %v", err)
	}
}
