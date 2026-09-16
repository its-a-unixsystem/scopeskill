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
)

func TestBuchungFileAddDryRunEncodesLocalFileWithoutRequest(t *testing.T) {
	localFile := filepath.Join(t.TempDir(), "invoice.pdf")
	if err := os.WriteFile(localFile, []byte("invoice bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, stderr := withCLI(t, "", false)

	if err := run([]string{"--config", postingConfigPath(t, "http://127.0.0.1:1"), "buchung", "file", "add", "2026-000051", localFile, "--dry-run"}); err != nil {
		t.Fatal(err)
	}

	status := stdoutStatus(t, output.String())
	if status["status"] != "dry_run" || status["documentNumber"] != "2026-000051" || status["filename"] != "invoice.pdf" {
		t.Fatalf("stdout = %s", output.String())
	}
	for _, want := range []string{
		"POST /journal/2026-000051/file/new",
		`"filename": "invoice.pdf"`,
		`"data": "` + base64.StdEncoding.EncodeToString([]byte("invoice bytes")) + `"`,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("preview missing %q in %s", want, stderr.String())
		}
	}
}

func TestBuchungFileAddPostsFileFormOnce(t *testing.T) {
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/journal/2026-000051/file/new":
			writes++
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["filename"] != "invoice.pdf" || payload["data"] != base64.StdEncoding.EncodeToString([]byte("invoice bytes")) {
				t.Fatalf("payload = %#v", payload)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	localFile := filepath.Join(t.TempDir(), "invoice.pdf")
	if err := os.WriteFile(localFile, []byte("invoice bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", postingConfigPath(t, server.URL), "buchung", "file", "add", "2026-000051", localFile, "--yes"}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatalf("writes = %d", writes)
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "added" || status["documentNumber"] != "2026-000051" || status["filename"] != "invoice.pdf" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestBuchungFileGetUsesResponseFilename(t *testing.T) {
	server := journalFileServer(t, "/rest/journal/2026-000051/file", `attachment; filename="supplier-invoice.pdf"`)
	defer server.Close()
	dir := t.TempDir()
	t.Chdir(dir)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", postingConfigPath(t, server.URL), "buchung", "file", "get", "2026-000051"}); err != nil {
		t.Fatal(err)
	}
	assertDownloadedFile(t, filepath.Join(dir, "supplier-invoice.pdf"))
	if strings.TrimSpace(output.String()) != "supplier-invoice.pdf" {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestBuchungFileGetWithStampUsesExplicitOutput(t *testing.T) {
	server := journalFileServer(t, "/rest/journal/2026-000051/filewithstamp", "")
	defer server.Close()
	out := filepath.Join(t.TempDir(), "stamped.pdf")
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", postingConfigPath(t, server.URL), "buchung", "file", "get", "2026-000051", "--out", out, "--with-stamp"}); err != nil {
		t.Fatal(err)
	}
	assertDownloadedFile(t, out)
	if strings.TrimSpace(output.String()) != out {
		t.Fatalf("stdout = %q", output.String())
	}
}

func journalFileServer(t *testing.T, wantPath, disposition string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case wantPath:
			if r.Method != http.MethodGet {
				t.Fatalf("method = %s", r.Method)
			}
			if disposition != "" {
				w.Header().Set("Content-Disposition", disposition)
			}
			_, _ = io.WriteString(w, "invoice bytes")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func assertDownloadedFile(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "invoice bytes" {
		t.Fatalf("downloaded = %q", raw)
	}
}
