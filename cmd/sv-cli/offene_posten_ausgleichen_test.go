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

type offenePostenAusgleichenStub struct {
	server *httptest.Server
	paths  []string
	bodies []map[string]any
	status int
}

func newOffenePostenAusgleichenStub(t *testing.T) *offenePostenAusgleichenStub {
	t.Helper()
	stub := &offenePostenAusgleichenStub{status: http.StatusOK}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/openitems/creditor/clearing", "/rest/openitems/debitor/clearing":
			stub.paths = append(stub.paths, r.URL.Path)
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.bodies = append(stub.bodies, body)
			if stub.status != http.StatusOK {
				http.Error(w, "rejected", stub.status)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func clearingFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clearing.json")
	raw := []byte(`{
		"clearings": [{
			"documentNumber": "PAY-2025-7",
			"documents": [{"documentNumber": "INV-2025-42", "clearingAmount": 119.00}]
		}]
	}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOffenePostenAusgleichenDryRunPreviewsWithoutWrite(t *testing.T) {
	stub := newOffenePostenAusgleichenStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	dataPath := clearingFixture(t)
	output, stderr := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "ausgleichen", "--seite=kreditor", "--data", "@" + dataPath, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if len(stub.paths) != 0 {
		t.Fatalf("write paths = %#v", stub.paths)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "dry_run" || got["seite"] != "kreditor" || got["endpoint"] != "POST /openitems/creditor/clearing" {
		t.Fatalf("stdout = %s", output.String())
	}
	for _, want := range []string{"POST /openitems/creditor/clearing", "PAY-2025-7", "INV-2025-42", "119"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("preview missing %q in %q", want, stderr.String())
		}
	}
}

func TestOffenePostenAusgleichenRoutesBothSidesAndWritesOnce(t *testing.T) {
	for _, tc := range []struct {
		seite string
		path  string
	}{
		{seite: "kreditor", path: "/rest/openitems/creditor/clearing"},
		{seite: "debitor", path: "/rest/openitems/debitor/clearing"},
	} {
		t.Run(tc.seite, func(t *testing.T) {
			stub := newOffenePostenAusgleichenStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			dataPath := clearingFixture(t)
			output, _ := withCLI(t, "", false)

			if err := run([]string{"--config", configPath, "offene-posten", "ausgleichen", "--seite=" + tc.seite, "--data", "@" + dataPath, "--yes"}); err != nil {
				t.Fatal(err)
			}
			if len(stub.paths) != 1 || stub.paths[0] != tc.path {
				t.Fatalf("write paths = %#v", stub.paths)
			}
			got := stdoutStatus(t, output.String())
			if got["status"] != "cleared" || got["seite"] != tc.seite {
				t.Fatalf("stdout = %s", output.String())
			}
			clearings, ok := stub.bodies[0]["clearings"].([]any)
			if !ok || len(clearings) != 1 {
				t.Fatalf("body = %#v", stub.bodies[0])
			}
		})
	}
}

func TestOffenePostenAusgleichenRejectsInvalidPayloadBeforeWrite(t *testing.T) {
	stub := newOffenePostenAusgleichenStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"clearings":[{"documentNumber":"PAY-1","documents":[]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "offene-posten", "ausgleichen", "--seite=kreditor", "--data", "@" + path, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "documents") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.paths) != 0 {
		t.Fatalf("write paths = %#v", stub.paths)
	}
}

func TestOffenePostenAusgleichenDoesNotRetryRejectedWrite(t *testing.T) {
	stub := newOffenePostenAusgleichenStub(t)
	stub.status = http.StatusBadRequest
	configPath := sachkontoConfigPath(t, stub.server.URL)
	dataPath := clearingFixture(t)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "offene-posten", "ausgleichen", "--seite=debitor", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.paths) != 1 {
		t.Fatalf("write paths = %#v", stub.paths)
	}
}
