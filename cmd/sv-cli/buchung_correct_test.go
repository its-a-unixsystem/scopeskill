package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuchungCorrectionCommandsAreGatedWithoutRequests(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits++
	}))
	defer server.Close()
	config := postingConfigPath(t, server.URL)

	tests := []struct {
		name     string
		args     []string
		command  string
		endpoint string
	}{
		{
			name:     "correct",
			args:     []string{"buchung", "correct", "P-2025-1", "--file=@does-not-exist", "--yes"},
			command:  "buchung correct",
			endpoint: "POST /correctpostings",
		},
		{
			name:     "correct-import",
			args:     []string{"buchung", "correct-import", "--file=does-not-exist", "--yes"},
			command:  "buchung correct-import",
			endpoint: "POST /postings/correction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, _ := withCLI(t, "", false)
			err := run(append([]string{"--config", config}, tt.args...))
			if err == nil || !strings.Contains(err.Error(), "controlled live contract test") {
				t.Fatalf("error = %v", err)
			}
			got := stdoutStatus(t, output.String())
			if got["status"] != "conflict" || got["state"] != "conflict" || got["command"] != tt.command || got["endpoint"] != tt.endpoint {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}

	if hits != 0 {
		t.Fatalf("requests = %d", hits)
	}
}
