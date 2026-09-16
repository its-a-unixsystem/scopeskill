package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

func TestBerichtShowPassesProReportResponseThrough(t *testing.T) {
	const report = `{"columns":["Januar"],"rows":[{"name":"Rohertrag","value":1234.56}]}`
	var requestPath string
	var requestQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		default:
			requestPath = r.URL.Path
			requestQuery = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, report)
		}
	}))
	defer server.Close()
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", berichtConfigPath(t, server.URL),
		"bericht", "show",
		"--type=bwa", "--name=Standard BWA", "--year=2026", "--month=1",
		"--show-accounts", "--layout=Monatsvergleich",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/rest/proreport/3/Standard BWA/2026/1" {
		t.Fatalf("path = %q", requestPath)
	}
	wantQuery := url.Values{"layoutName": {"Monatsvergleich"}, "showAccounts": {"true"}}
	if requestQuery.Encode() != wantQuery.Encode() {
		t.Fatalf("query = %q, want %q", requestQuery.Encode(), wantQuery.Encode())
	}
	if output.String() != report {
		t.Fatalf("stdout = %q, want raw report", output.String())
	}
}

func TestBerichtShowUsesStandardColumnLayoutByDefault(t *testing.T) {
	var layoutName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		default:
			layoutName = r.URL.Query().Get("layoutName")
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	withCLI(t, "", false)

	err := run([]string{
		"--config", berichtConfigPath(t, server.URL),
		"bericht", "show",
		"--type=guv", "--name=Standard", "--year=2026", "--month=9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if layoutName != "Standard" {
		t.Fatalf("layoutName = %q, want Standard", layoutName)
	}
}

func TestBerichtExportDownloadsReportBytes(t *testing.T) {
	const exported = "%PDF-1.7 report bytes"
	var requestPath string
	var requestQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		default:
			requestPath = r.URL.Path
			requestQuery = r.URL.Query()
			_, _ = io.WriteString(w, exported)
		}
	}))
	defer server.Close()
	out := filepath.Join(t.TempDir(), "bilanz.pdf")
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", berichtConfigPath(t, server.URL),
		"bericht", "export",
		"--type=bilanz", "--from=01.01.2026", "--to=31.12.2026",
		"--format=pdf", "--out", out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/rest/reports/bilanz" {
		t.Fatalf("path = %q", requestPath)
	}
	wantQuery := url.Values{
		"startDate":    {"01.01.2026"},
		"endDate":      {"31.12.2026"},
		"outputFormat": {"pdf"},
	}
	if requestQuery.Encode() != wantQuery.Encode() {
		t.Fatalf("query = %q, want %q", requestQuery.Encode(), wantQuery.Encode())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != exported {
		t.Fatalf("exported bytes = %q", raw)
	}
	if output.String() != out+"\n" {
		t.Fatalf("stdout = %q", output.String())
	}
}

func berichtConfigPath(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	raw := []byte("BASE_URL=" + baseURL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return path
}
