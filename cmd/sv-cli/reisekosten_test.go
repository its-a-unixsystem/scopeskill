package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

func TestReisekostenListPostsSearchBody(t *testing.T) {
	const fixture = `{"search":[{"field":"fixtureDate","operator":"greaterorequal","value":"2026-09-01"},{"field":"fixtureDate","operator":"lessorequal","value":"2026-09-30"}],"page":0,"pageSize":25}`
	var method, path string
	var body []byte
	var handlerErrors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/travelentries":
			method = r.Method
			path = r.URL.Path
			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				handlerErrors = append(handlerErrors, err.Error())
				http.Error(w, "request body unavailable", http.StatusInternalServerError)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"id": 100021}}, "total": 1})
		default:
			handlerErrors = append(handlerErrors, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	dataPath := filepath.Join(t.TempDir(), "travel-search.json")
	if err := os.WriteFile(dataPath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", postingConfigPath(t, server.URL), "reisekosten", "list", "--data", "@" + dataPath}); err != nil {
		t.Fatal(err)
	}

	if len(handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", handlerErrors)
	}
	if method != http.MethodPost || path != "/rest/travelentries" {
		t.Fatalf("request = %s %s", method, path)
	}
	var gotBody, wantBody any
	if err := json.Unmarshal(body, &gotBody); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if err := json.Unmarshal([]byte(fixture), &wantBody); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Fatalf("request body = %s", body)
	}
	var gotOutput map[string]any
	if err := json.Unmarshal(output.Bytes(), &gotOutput); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if gotOutput["total"] != float64(1) {
		t.Fatalf("stdout = %s", output.String())
	}
	records, ok := gotOutput["records"].([]any)
	if !ok || len(records) != 1 || records[0].(map[string]any)["id"] != float64(100021) {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestReisekostenListDefaultOutput(t *testing.T) {
	var requestCount int
	var handlerErrors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/travelentries":
			requestCount++
			if r.Method != http.MethodPost {
				handlerErrors = append(handlerErrors, "method = "+r.Method)
				http.Error(w, "unexpected method", http.StatusInternalServerError)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": []any{}})
		default:
			handlerErrors = append(handlerErrors, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", postingConfigPath(t, server.URL), "reisekosten", "list"}); err != nil {
		t.Fatal(err)
	}

	if len(handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", handlerErrors)
	}
	if requestCount != 1 {
		t.Fatalf("requests = %d", requestCount)
	}
	if strings.TrimSpace(output.String()) != "[]" {
		t.Fatalf("stdout = %q", output.String())
	}
}

type reisekostenCreateObservation struct {
	requests      int
	mutations     int
	method        string
	path          string
	contentType   string
	body          []byte
	handlerErrors []string
}

func newReisekostenCreateServer(t *testing.T, endpoint string, status int, response string) (*httptest.Server, *reisekostenCreateObservation) {
	t.Helper()
	observation := &reisekostenCreateObservation{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observation.requests++
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case endpoint:
			observation.mutations++
			observation.method = r.Method
			observation.path = r.URL.Path
			observation.contentType = r.Header.Get("Content-Type")
			var err error
			observation.body, err = io.ReadAll(r.Body)
			if err != nil {
				observation.handlerErrors = append(observation.handlerErrors, err.Error())
				http.Error(w, "request body unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if response != "" {
				_, _ = io.WriteString(w, response)
			}
		default:
			observation.handlerErrors = append(observation.handlerErrors, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server, observation
}

func reisekostenFixture(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "position.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReisekostenCreateRoutesByKind(t *testing.T) {
	testCases := []struct {
		name     string
		kind     string
		endpoint string
		fixture  string
		response string
	}{
		{
			name:     "Nebenkosten",
			kind:     "nebenkosten",
			endpoint: "/rest/travelentry/position/extra/new",
			fixture:  `{"travelEntryId":100021,"fileform":[],"amount":53,"taxRate":19,"typeDisplay":"Parken"}`,
			response: `{"id":7001}`,
		},
		{
			name:     "Übernachtung",
			kind:     "uebernachtung",
			endpoint: "/rest/travelentry/position/overnight/new",
			fixture:  `{"travelEntryId":100021,"fileform":[],"amount":120,"taxRate":7}`,
		},
		{
			name:     "Fahrtkosten",
			kind:     "fahrtkosten",
			endpoint: "/rest/travelentry/position/vehicle/new",
			fixture:  `{"travelEntryId":9007199254740993,"fileform":[],"distance":50,"typeDisplay":"Eigener PKW"}`,
			response: `{"id":7001}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, observation := newReisekostenCreateServer(t, testCase.endpoint, http.StatusCreated, testCase.response)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			output, _ := withCLI(t, "", false)

			err := run([]string{
				"--config", postingConfigPath(t, server.URL),
				"reisekosten", testCase.kind, "create",
				"--file=" + reisekostenFixture(t, testCase.fixture), "--yes",
			})
			if err != nil {
				t.Fatal(err)
			}

			if len(observation.handlerErrors) != 0 {
				t.Fatalf("handler errors = %v", observation.handlerErrors)
			}
			if observation.mutations != 1 || observation.method != http.MethodPost || observation.path != testCase.endpoint {
				t.Fatalf("mutation = %d %s %s", observation.mutations, observation.method, observation.path)
			}
			if observation.contentType != "application/json" {
				t.Fatalf("Content-Type = %q", observation.contentType)
			}
			var gotBody, wantBody map[string]json.RawMessage
			if err := json.Unmarshal(observation.body, &gotBody); err != nil {
				t.Fatalf("request body: %v", err)
			}
			if err := json.Unmarshal([]byte(testCase.fixture), &wantBody); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotBody, wantBody) {
				t.Fatalf("request body = %s", observation.body)
			}
			status := stdoutStatus(t, output.String())
			if status["status"] != "created" || status["endpoint"] != "POST "+strings.TrimPrefix(testCase.endpoint, "/rest") {
				t.Fatalf("stdout = %s", output.String())
			}
			response, present := status["response"]
			if !present {
				t.Fatalf("stdout response is absent: %s", output.String())
			}
			if testCase.response == "" {
				if response != nil {
					t.Fatalf("stdout = %s", output.String())
				}
			} else if !reflect.DeepEqual(response, map[string]any{"id": float64(7001)}) {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}

func TestReisekostenCreateDryRunDoesNotCallAPI(t *testing.T) {
	const fixture = `{"travelEntryId":100021,"fileform":[],"amount":53}`
	server, observation := newReisekostenCreateServer(t, "/rest/travelentry/position/extra/new", http.StatusCreated, "")
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", postingConfigPath(t, server.URL),
		"reisekosten", "nebenkosten", "create",
		"--file=" + reisekostenFixture(t, fixture), "--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.requests != 0 {
		t.Fatalf("requests = %d", observation.requests)
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "dry_run" || status["endpoint"] != "POST /travelentry/position/extra/new" {
		t.Fatalf("stdout = %s", output.String())
	}
	request, ok := status["request"].(map[string]any)
	if !ok || request["travelEntryId"] != float64(100021) || request["amount"] != float64(53) {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestReisekostenCreateRejectsInvalidFile(t *testing.T) {
	testCases := []struct {
		name string
		args func(*testing.T) []string
	}{
		{name: "missing file", args: func(*testing.T) []string { return nil }},
		{name: "nonexistent file", args: func(t *testing.T) []string {
			return []string{"--file=" + filepath.Join(t.TempDir(), "missing.json")}
		}},
		{name: "malformed JSON", args: func(t *testing.T) []string {
			return []string{"--file=" + reisekostenFixture(t, `{`)}
		}},
		{name: "array", args: func(t *testing.T) []string {
			return []string{"--file=" + reisekostenFixture(t, `[]`)}
		}},
		{name: "null", args: func(t *testing.T) []string {
			return []string{"--file=" + reisekostenFixture(t, `null`)}
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, observation := newReisekostenCreateServer(t, "/rest/travelentry/position/extra/new", http.StatusCreated, "")
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			withCLI(t, "", false)
			args := []string{"--config", postingConfigPath(t, server.URL), "reisekosten", "nebenkosten", "create"}
			args = append(args, testCase.args(t)...)

			if err := run(args); err == nil {
				t.Fatal("expected error")
			}
			if observation.requests != 0 {
				t.Fatalf("requests = %d", observation.requests)
			}
		})
	}
}

func TestReisekostenCreateRequiresConfirmation(t *testing.T) {
	const fixture = `{"travelEntryId":100021,"fileform":[]}`
	server, observation := newReisekostenCreateServer(t, "/rest/travelentry/position/extra/new", http.StatusCreated, `{"id":7001}`)
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	configPath := postingConfigPath(t, server.URL)
	filePath := reisekostenFixture(t, fixture)

	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "reisekosten", "nebenkosten", "create", "--file=" + filePath})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("error = %v", err)
	}
	if observation.requests != 0 {
		t.Fatalf("requests before confirmation = %d", observation.requests)
	}

	output, _ := withCLI(t, "create nebenkosten position "+filepath.Base(filePath)+"\n", true)
	err = run([]string{"--config", configPath, "reisekosten", "nebenkosten", "create", "--file=" + filePath})
	if err != nil {
		t.Fatal(err)
	}
	if observation.mutations != 1 {
		t.Fatalf("mutations = %d", observation.mutations)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "created" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestReisekostenCreatePreservesAPIErrors(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "not found", status: http.StatusNotFound, body: "travel entry unavailable"},
		{name: "server failure", status: http.StatusInternalServerError, body: "temporary failure"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, observation := newReisekostenCreateServer(t, "/rest/travelentry/position/extra/new", testCase.status, testCase.body)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			output, _ := withCLI(t, "", false)
			err := run([]string{
				"--config", postingConfigPath(t, server.URL),
				"reisekosten", "nebenkosten", "create",
				"--file=" + reisekostenFixture(t, `{"travelEntryId":100021,"fileform":[]}`), "--yes",
			})

			if err == nil || !strings.Contains(err.Error(), testCase.body) {
				t.Fatalf("error = %v", err)
			}
			if observation.mutations != 1 {
				t.Fatalf("mutations = %d", observation.mutations)
			}
			if strings.Contains(output.String(), "created") {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}
