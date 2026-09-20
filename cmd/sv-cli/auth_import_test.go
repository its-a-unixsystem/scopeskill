package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

func TestAuthImportCreatesConfigWithExplicitSKR(t *testing.T) {
	var refreshGrant url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		refreshGrant = r.PostForm
		writeJSONForCLI(w, map[string]any{
			"token_type":   "Bearer",
			"access_token": "access-from-refresh",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "scopeskill", "config")
	t.Setenv(scopeskill.EnvBaseURL, server.URL)
	output, stderr := withCLI(t, "1234567\nvendor-refresh-token\n", true)

	if err := run([]string{"--config", path, "auth", "import", "--skr=skr04"}); err != nil {
		t.Fatal(err)
	}
	if refreshGrant.Get("grant_type") != "refresh_token" || refreshGrant.Get("customer") != "1234567" || refreshGrant.Get("refresh_token") != "vendor-refresh-token" {
		t.Fatalf("refresh grant = %v", refreshGrant)
	}
	if !strings.Contains(stderr.String(), "Kundennummer:") || !strings.Contains(stderr.String(), "Refresh Token: ********************") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "vendor-refresh-token") {
		t.Fatalf("stderr leaked refresh token: %q", stderr.String())
	}
	if !strings.Contains(output.String(), "scopeskill config written") {
		t.Fatalf("output = %q", output.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CUSTOMER=1234567", "REST_REFRESH_TOKEN=vendor-refresh-token", "SKR=skr04"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config missing %q in %s", want, raw)
		}
	}
	assertMode(t, filepath.Dir(path), 0o700)
	assertMode(t, path, 0o600)
}

func TestAuthImportProbesSKRAndPreservesConfig(t *testing.T) {
	server := loginProbeServer(t, map[string]bool{"8400": true}, nil)
	path := filepath.Join(t.TempDir(), "config")
	old := "# keep this comment\nBASE_URL=" + server.URL + "\nUNKNOWN=survives\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	withCLI(t, "1234567\nvendor-refresh-token\n", true)

	if err := run([]string{"--config", path, "auth", "import"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CUSTOMER=1234567", "REST_REFRESH_TOKEN=vendor-refresh-token", "SKR=skr03", "# keep this comment", "UNKNOWN=survives"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("config missing %q in %s", want, raw)
		}
	}
}

func TestAuthImportRefusesOverwriteUnlessForced(t *testing.T) {
	var refreshCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCount++
		writeJSONForCLI(w, map[string]any{"access_token": "new-access-token"})
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config")
	old := "BASE_URL=" + server.URL + "\nCUSTOMER=old\nREST_REFRESH_TOKEN=old-token\nSKR=skr03\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	withCLI(t, "1234567\nnew-token\n", true)

	err := run([]string{"--config", path, "auth", "import", "--skr=skr04"})
	if err == nil || !strings.Contains(err.Error(), "auth import --force") {
		t.Fatalf("error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != old || refreshCount != 0 {
		t.Fatalf("config or API changed before --force: config=%q requests=%d", raw, refreshCount)
	}

	withCLI(t, "1234567\nnew-token\n", true)
	if err := run([]string{"--config", path, "auth", "import", "--force", "--skr=skr04"}); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "REST_REFRESH_TOKEN=new-token") || !strings.Contains(string(raw), "SKR=skr04") || refreshCount != 1 {
		t.Fatalf("forced import: config=%q requests=%d", raw, refreshCount)
	}
}

func TestAuthImportInvalidCredentialsLeaveConfigUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config")
	old := "# keep\nBASE_URL=" + server.URL + "\nCUSTOMER=old\nREST_REFRESH_TOKEN=old-token\nSKR=skr03\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	withCLI(t, "wrong-customer\nwrong-token\n", true)

	err := run([]string{"--config", path, "auth", "import", "--force"})
	if err == nil || !strings.Contains(err.Error(), "verify Kundennummer and Refresh Token") {
		t.Fatalf("error = %v", err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != old {
		t.Fatalf("config was modified:\n%s", raw)
	}
}

func TestAuthImportRejectsEmptyInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "customer", input: "\nrefresh-token\n", want: "Kundennummer ist erforderlich"},
		{name: "refresh token", input: "1234567\n\n", want: "Refresh Token ist erforderlich"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			withCLI(t, tc.input, true)
			err := run([]string{"--config", path, "auth", "import", "--skr=skr04"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
			if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
				t.Fatalf("config stat error = %v", statErr)
			}
		})
	}
}
