package main

import (
	"encoding/base64"
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

func TestDATEVExportPassesResponseBytesThrough(t *testing.T) {
	const exported = "EXTF;700;21;Buchungsstapel\r\nUmsatz;Soll/Haben-Kennzeichen\r\n"
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/datevexport":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, exported)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "export.csv")
	output, _ := withCLI(t, "", false)
	err := run([]string{
		"--config", datevConfigPath(t, server.URL),
		"datev", "export",
		"--from=2026-01-01", "--to=2026-12-31", "--out=" + out, "--fiscal-year=2026",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := requestBody["fromPostingDateTs"]; got != float64(1767222000000) {
		t.Fatalf("fromPostingDateTs = %#v", got)
	}
	if got := requestBody["toPostingDateTs"]; got != float64(1798757999999) {
		t.Fatalf("toPostingDateTs = %#v", got)
	}
	if got := requestBody["fiscalYear"]; got != float64(2026) {
		t.Fatalf("fiscalYear = %#v", got)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != exported {
		t.Fatalf("exported bytes = %q", raw)
	}
	if strings.TrimSpace(output.String()) != out {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestDATEVImportPassesFileBytesThrough(t *testing.T) {
	datevBytes := []byte("EXTF;700;21;Buchungsstapel\r\n\xff\x00;unparsed\r\n")
	file := filepath.Join(t.TempDir(), "datev.csv")
	if err := os.WriteFile(file, datevBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/datevpostings/new":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"imported":true}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", datevConfigPath(t, server.URL), "datev", "import", "--file=" + file, "--yes"}); err != nil {
		t.Fatal(err)
	}
	encoded, ok := requestBody["data"].(string)
	if !ok {
		t.Fatalf("request body = %#v", requestBody)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(datevBytes) {
		t.Fatalf("imported bytes = %q", decoded)
	}
	if output.String() != "{\"imported\":true}" {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestDATEVImportDryRunDoesNotCallAPI(t *testing.T) {
	file := filepath.Join(t.TempDir(), "datev.csv")
	if err := os.WriteFile(file, []byte("raw DATEV EXTF"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := scopeskill.NewClient(scopeskill.Config{AccessToken: "access", BaseURL: "http://127.0.0.1:1"})
	output, _ := withCLI(t, "", false)
	if err := datevImport(client, []string{"--file=" + file, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status": "dry_run"`) {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestDATEVImportSurfacesRawAPIValidationError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invalid.csv")
	if err := os.WriteFile(file, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	const validationError = `{"code":"DATEV_VALIDATION","message":"Spalte 7 ist ungültig"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/datevpostings/new":
			http.Error(w, validationError, http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	withCLI(t, "", false)

	err := run([]string{"--config", datevConfigPath(t, server.URL), "datev", "import", "--file=" + file, "--yes"})
	if err == nil || !strings.Contains(err.Error(), validationError) {
		t.Fatalf("error = %v", err)
	}
}

func datevConfigPath(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	raw := []byte("BASE_URL=" + baseURL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return path
}
