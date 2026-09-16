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

func correctionFixture(t *testing.T, value any, name string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func correctionServer(t *testing.T, wantPath string, wantBody func([]byte)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "test", "expires_in": 3600})
		case (r.URL.Path == wantPath || r.URL.Path == strings.TrimPrefix(wantPath, "/rest")) && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			wantBody(raw)
			writeJSONForCLI(w, map[string]any{"status": 201, "insertCount": "1", "updateCount": "0"})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && r.Method == http.MethodGet:
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"documentNumber": "P-2025-1", "postingDate": "2025-06-02", "pdeRowNumber": "1", "account": "4400", "amount": "119.00"}}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
}

func TestBuchungCorrectSendsExpectedJSON(t *testing.T) {
	input := validPostingInput
	server := correctionServer(t, "/rest/correctpostings", func(raw []byte) {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		rows := body["rows"].([]any)
		if len(rows) != 2 || rows[0].(map[string]any)["documentNumber"] != "P-2025-1" {
			t.Fatalf("body = %s", raw)
		}
	})
	defer server.Close()
	config := postingConfigPath(t, server.URL)
	file := correctionFixture(t, input, "correction.json")
	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", config, "buchung", "correct", "P-2025-1", "--file=@" + file, "--yes"}); err != nil {
		t.Fatalf("correct: %v", err)
	}
	if stdoutStatus(t, output.String())["status"] != "corrected" {
		t.Fatal(output.String())
	}
}

func TestBuchungCorrectImportSendsExpectedJSON(t *testing.T) {
	server := correctionServer(t, "/rest/postings/correction", func(raw []byte) {
		var body []any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 {
			t.Fatalf("body = %s", raw)
		}
	})
	defer server.Close()
	config := postingConfigPath(t, server.URL)
	file := correctionFixture(t, []any{validPostingInput}, "corrections.json")
	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", config, "buchung", "correct-import", "--file=" + file, "--yes"}); err != nil {
		t.Fatal(err)
	}
	if stdoutStatus(t, output.String())["status"] != "corrected" {
		t.Fatal(output.String())
	}
}
