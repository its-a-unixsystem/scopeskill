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
)

func buchungUpdateServer(t *testing.T, onPut func(body map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "test", "expires_in": 3600})
		case (r.URL.Path == "/rest/postings/update" || r.URL.Path == "/postings/update") && r.Method == http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("unmarshal PUT body: %v", err)
			}
			if onPut != nil {
				onPut(body)
			}
			writeJSONForCLI(w, map[string]any{"status": 200, "message": "Successfully updated"})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && r.Method == http.MethodGet:
			docNum := strings.TrimPrefix(r.URL.Path, "/rest/journal/")
			writeJSONForCLI(w, map[string]any{
				"records": []any{
					map[string]any{
						"documentNumber": docNum,
						"postingDate":    "2026-01-15",
						"amount":         "150.00",
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func TestBuchungUpdateDryRun(t *testing.T) {
	putHit := false
	server := buchungUpdateServer(t, func(body map[string]any) {
		putHit = true
	})
	defer server.Close()

	config := postingConfigPath(t, server.URL)
	stdout, stderr := withCLI(t, "", false)

	err := run([]string{
		"--config", config,
		"buchung", "update", "DOC-100",
		"--internal-number=INT-100",
		"--external-number=EXT-100",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if putHit {
		t.Fatal("PUT request was made during dry-run")
	}

	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("parse stdout: %v: %s", err, stdout.String())
	}
	if res["status"] != "dry_run" || res["documentNumber"] != "DOC-100" {
		t.Fatalf("unexpected stdout: %v", res)
	}
	if !strings.Contains(stderr.String(), "sv-cli buchung update preview") {
		t.Fatalf("stderr missing preview: %s", stderr.String())
	}
}

func TestBuchungUpdateSuccess(t *testing.T) {
	var capturedBody map[string]any
	server := buchungUpdateServer(t, func(body map[string]any) {
		capturedBody = body
	})
	defer server.Close()

	config := postingConfigPath(t, server.URL)
	stdout, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", config,
		"buchung", "update", "DOC-200",
		"--internal-number=INT-200",
		"--external-number=EXT-200",
		"--yes",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if capturedBody["documentNumber"] != "DOC-200" ||
		capturedBody["internalDocumentNumber"] != "INT-200" ||
		capturedBody["externalDocumentNumber"] != "EXT-200" {
		t.Fatalf("unexpected PUT body: %#v", capturedBody)
	}

	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("parse stdout: %v: %s", err, stdout.String())
	}
	if res["status"] != "updated" || res["documentNumber"] != "DOC-200" {
		t.Fatalf("unexpected stdout: %v", res)
	}
	if res["buchung"] == nil {
		t.Fatalf("expected readback buchung in stdout: %v", res)
	}
}

func TestBuchungUpdateNumbersAlias(t *testing.T) {
	called := false
	server := buchungUpdateServer(t, func(body map[string]any) {
		called = true
	})
	defer server.Close()

	config := postingConfigPath(t, server.URL)
	stdout, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", config,
		"buchung", "update-numbers", "DOC-250",
		"--external-number=EXT-250",
		"--yes",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !called {
		t.Fatal("expected PUT via update-numbers alias")
	}
	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("parse stdout: %v", err)
	}
	if res["status"] != "updated" {
		t.Fatalf("unexpected status: %v", res["status"])
	}
}

func TestBuchungUpdateWithFile(t *testing.T) {
	var capturedBody map[string]any
	server := buchungUpdateServer(t, func(body map[string]any) {
		capturedBody = body
	})
	defer server.Close()

	filePayload := map[string]any{
		"documentNumber":         "DOC-300",
		"externalDocumentNumber": "EXT-300-FILE",
	}
	raw, _ := json.Marshal(filePayload)
	filePath := filepath.Join(t.TempDir(), "update.json")
	if err := os.WriteFile(filePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	config := postingConfigPath(t, server.URL)
	stdout, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", config,
		"buchung", "update", "DOC-300",
		"--file=" + filePath,
		"--yes",
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if capturedBody["documentNumber"] != "DOC-300" ||
		capturedBody["externalDocumentNumber"] != "EXT-300-FILE" {
		t.Fatalf("unexpected PUT body: %#v", capturedBody)
	}

	var res map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("parse stdout: %v: %s", err, stdout.String())
	}
	if res["status"] != "updated" {
		t.Fatalf("unexpected stdout status: %v", res)
	}
}

func TestBuchungUpdateValidation(t *testing.T) {
	server := buchungUpdateServer(t, nil)
	defer server.Close()
	config := postingConfigPath(t, server.URL)

	// Missing document number
	if err := run([]string{"--config", config, "buchung", "update"}); err == nil {
		t.Fatal("expected error on missing documentNumber")
	}

	// No modification flags
	if err := run([]string{"--config", config, "buchung", "update", "DOC-1", "--yes"}); err == nil {
		t.Fatal("expected error on no modification flags")
	}

	// Non-TTY without --yes
	_, _ = withCLI(t, "", false)
	err := run([]string{
		"--config", config,
		"buchung", "update", "DOC-1",
		"--internal-number=INT-1",
	})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY or --yes") {
		t.Fatalf("expected non-interactive error, got: %v", err)
	}
}
