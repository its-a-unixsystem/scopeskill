package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopevisio"
)

func TestParseGlobalFlagsConfig(t *testing.T) {
	configPath, args, err := parseGlobalFlags([]string{"--config", "/tmp/scopevisio.env", "get", "/myaccount"})
	if err != nil {
		t.Fatal(err)
	}
	if configPath != "/tmp/scopevisio.env" {
		t.Fatalf("config path = %q", configPath)
	}
	if len(args) != 2 || args[0] != "get" || args[1] != "/myaccount" {
		t.Fatalf("args = %#v", args)
	}
}

func TestAuthHelpListsLogin(t *testing.T) {
	output, _ := withCLI(t, "", false)
	if err := run([]string{"auth"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "login") {
		t.Fatalf("auth help = %q", output.String())
	}
}

func TestAuthLoginWritesConfigAndGetUsesIt(t *testing.T) {
	var passwordGrant url.Values
	var refreshGrant url.Values
	var myAccountAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			switch r.PostForm.Get("grant_type") {
			case "password":
				passwordGrant = r.PostForm
				writeJSONForCLI(w, map[string]any{
					"token_type":    "Bearer",
					"access_token":  "access-from-login",
					"refresh_token": "refresh-from-login",
					"expires_in":    3600,
				})
			case "refresh_token":
				refreshGrant = r.PostForm
				writeJSONForCLI(w, map[string]any{
					"token_type":   "Bearer",
					"access_token": "access-from-refresh",
					"expires_in":   3600,
				})
			default:
				t.Fatalf("grant_type = %q", r.PostForm.Get("grant_type"))
			}
		case "/rest/myaccount":
			myAccountAuth = r.Header.Get("Authorization")
			writeJSONForCLI(w, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv(scopevisio.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))

	path := filepath.Join(t.TempDir(), "scopeskill", "config")
	raw := strings.Join([]string{
		"# keep this comment",
		"BASE_URL=" + server.URL,
		"UNKNOWN=survives",
		"",
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	output, stderr := withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", true)
	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Scopevisio config written") {
		t.Fatalf("output = %q", output.String())
	}
	for _, prompt := range []string{"Customer number:", "Username:", "Password:", "Organisation ID:"} {
		if !strings.Contains(stderr.String(), prompt) {
			t.Fatalf("stderr missing prompt %q in %q", prompt, stderr.String())
		}
	}
	for key, want := range map[string]string{
		"grant_type":      "password",
		"customer":        "1234567",
		"username":        "tech@example.com",
		"password":        "secret-password",
		"organisation_id": "org-secret-id",
	} {
		if got := passwordGrant.Get(key); got != want {
			t.Fatalf("%s = %q", key, got)
		}
	}

	configRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	configText := string(configRaw)
	for _, want := range []string{
		scopevisio.AuthLoginConfigHeader,
		"CUSTOMER=1234567",
		"REST_REFRESH_TOKEN=refresh-from-login",
		"# keep this comment",
		"BASE_URL=" + server.URL,
		"UNKNOWN=survives",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config missing %q in %s", want, configText)
		}
	}
	for _, unwanted := range []string{"secret-password", "organisation_id", "org-secret-id"} {
		if strings.Contains(configText, unwanted) {
			t.Fatalf("config contains %q in %s", unwanted, configText)
		}
	}
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)

	if err := run([]string{"--config", path, "get", "/myaccount"}); err != nil {
		t.Fatal(err)
	}
	if refreshGrant.Get("refresh_token") != "refresh-from-login" {
		t.Fatalf("refresh_token = %q", refreshGrant.Get("refresh_token"))
	}
	if myAccountAuth != "Bearer access-from-refresh" {
		t.Fatalf("Authorization = %q", myAccountAuth)
	}
}

func TestGetWritesAccessTokenCacheOverridePath(t *testing.T) {
	var refreshCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			refreshCount++
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/myaccount":
			writeJSONForCLI(w, map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + server.URL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(t.TempDir(), "override", "foo.json")
	t.Setenv(scopevisio.EnvAccessTokenCache, cachePath)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "get", "/myaccount"}); err != nil {
		t.Fatal(err)
	}
	if refreshCount != 1 {
		t.Fatalf("refresh count = %d", refreshCount)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatal(err)
	}
	assertMode(t, filepath.Dir(cachePath), 0o700)
	assertMode(t, cachePath, 0o600)
}

func TestDownloadWritesGenericBinaryResponse(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		default:
			requestedPath = r.URL.Path
			if got := r.Header.Get("Accept"); got != "*/*" {
				t.Fatalf("Accept = %q", got)
			}
			_, _ = w.Write([]byte("document-bytes"))
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + server.URL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopevisio.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	outPath := filepath.Join(t.TempDir(), "doc.bin")
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "download", "/teamworkbridge/document/doc-1", "--out", outPath}); err != nil {
		t.Fatal(err)
	}
	if requestedPath != "/rest/teamworkbridge/document/doc-1" {
		t.Fatalf("requested path = %q", requestedPath)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "document-bytes" {
		t.Fatalf("downloaded = %q", string(raw))
	}
	if !strings.Contains(output.String(), outPath) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestTeamworkUploadCommandBuildsMultipartRequest(t *testing.T) {
	var contentType string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/teamworkbridge/documents":
			contentType = r.Header.Get("Content-Type")
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			body = string(raw)
			writeJSONForCLI(w, map[string]any{"id": "doc-1"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + server.URL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopevisio.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	localFilePath := filepath.Join(t.TempDir(), "invoice.pdf")
	if err := os.WriteFile(localFilePath, []byte("invoice"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "teamwork", "upload", localFilePath, "--collection", "collection-1", "--tag", "scopevisio-test"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"id": "doc-1"`) {
		t.Fatalf("output = %q", output.String())
	}
	if !strings.Contains(contentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q", contentType)
	}
	for _, want := range []string{
		`name="metadata"`,
		`name="document"; filename="invoice.pdf"`,
		`"filename":"invoice.pdf"`,
		`"size":7`,
		`"add-to-collection":["collection-1"]`,
		`"add-tag":["scopevisio-test"]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("multipart body missing %q in %s", want, body)
		}
	}
}

func TestTeamworkWithoutSubcommandPrintsHelpAndFails(t *testing.T) {
	output, _ := withCLI(t, "", false)
	client := scopevisio.NewClient(scopevisio.Config{AccessToken: "access-token"})

	err := teamwork(client, nil)
	if err == nil {
		t.Fatal("expected missing teamwork subcommand error")
	}
	if !strings.Contains(output.String(), "upload") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestTopLevelTeamworkUploadCommandIsRemoved(t *testing.T) {
	withCLI(t, "", false)

	err := run([]string{"teamwork-upload"})
	if err == nil || !strings.Contains(err.Error(), "unknown command: teamwork-upload") {
		t.Fatalf("error = %v", err)
	}
}

func TestAuthLoginUsesDefaultConfigPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeJSONForCLI(w, map[string]any{
			"token_type":    "Bearer",
			"access_token":  "access-from-login",
			"refresh_token": "refresh-from-login",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv(scopevisio.EnvBaseURL, server.URL)
	withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", true)
	if err := run([]string{"auth", "login"}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(configDir, "scopeskill", "config")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "REST_REFRESH_TOKEN=refresh-from-login") {
		t.Fatalf("default config = %s", string(raw))
	}
}

func TestAuthLoginRefusesOverwriteUnlessForced(t *testing.T) {
	var passwordGrantCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passwordGrantCount++
		writeJSONForCLI(w, map[string]any{
			"token_type":    "Bearer",
			"access_token":  "access-from-login",
			"refresh_token": "new-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config")
	oldRaw := []byte("BASE_URL=" + server.URL + "\nCUSTOMER=old\nREST_REFRESH_TOKEN=old-token\n")
	if err := os.WriteFile(path, oldRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", true)
	err := run([]string{"--config", path, "auth", "login"})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(oldRaw) {
		t.Fatalf("config was modified:\n%s", string(raw))
	}
	if passwordGrantCount != 0 {
		t.Fatalf("password grant count = %d", passwordGrantCount)
	}

	withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", true)
	if err := run([]string{"--config", path, "auth", "login", "--force"}); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "REST_REFRESH_TOKEN=new-token") {
		t.Fatalf("config = %s", string(raw))
	}
}

func TestAuthLoginWarnsWhenEnvRefreshTokenShadowsConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONForCLI(w, map[string]any{
			"token_type":    "Bearer",
			"access_token":  "access-from-login",
			"refresh_token": "refresh-from-login",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("BASE_URL="+server.URL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopevisio.EnvRestRefreshToken, "env-token")
	_, stderr := withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", true)
	if err := run([]string{"--config", path, "auth", "login"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), scopevisio.EnvRestRefreshToken) || !strings.Contains(stderr.String(), "shadow") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestAuthLoginRequiresTTY(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	withCLI(t, "1234567\ntech@example.com\nsecret-password\norg-secret-id\n", false)
	err := run([]string{"--config", path, "auth", "login"})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("config stat error = %v", statErr)
	}
}

func withCLI(t *testing.T, input string, terminal bool) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	inputFile, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inputFile.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := inputFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldInput := cliInput
	oldOutput := cliOutput
	oldError := cliError
	oldIsTerminal := isTerminal
	output := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cliInput = inputFile
	cliOutput = output
	cliError = stderr
	isTerminal = func(*os.File) bool { return terminal }
	t.Cleanup(func() {
		cliInput = oldInput
		cliOutput = oldOutput
		cliError = oldError
		isTerminal = oldIsTerminal
		_ = inputFile.Close()
	})
	return output, stderr
}

func writeJSONForCLI(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o", path, got)
	}
}
