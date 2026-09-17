package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type kasseObservation struct {
	requests      int
	mutations     int
	method        string
	path          string
	contentType   string
	body          []byte
	handlerErrors []string
}

func newKasseServer(t *testing.T, status int, response string) (*httptest.Server, *kasseObservation) {
	t.Helper()
	observation := &kasseObservation{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observation.requests++
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/cashbookentry/new":
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

func TestKasseCreatePostsCashbookEntry(t *testing.T) {
	const fixture = `{"cashbookId":9007199254740993,"documentDate":1788134400000,"externalDocumentNumber":"B878","freeText":"Büromaterial","creditAmount":23,"impersonalAccountNumber":"4930"}`
	server, observation := newKasseServer(t, http.StatusCreated, "")
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", postingConfigPath(t, server.URL),
		"kasse", "create", "--data", fixture, "--yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", observation.handlerErrors)
	}
	if observation.mutations != 1 || observation.method != http.MethodPost || observation.path != "/rest/cashbookentry/new" {
		t.Fatalf("mutation = %d %s %s", observation.mutations, observation.method, observation.path)
	}
	if observation.contentType != "application/json" {
		t.Fatalf("Content-Type = %q", observation.contentType)
	}
	var gotBody, wantBody map[string]json.RawMessage
	if err := json.Unmarshal(observation.body, &gotBody); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if err := json.Unmarshal([]byte(fixture), &wantBody); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Fatalf("request body = %s", observation.body)
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "created" || status["endpoint"] != "POST /cashbookentry/new" {
		t.Fatalf("stdout = %s", output.String())
	}
	if response, present := status["response"]; !present || response != nil {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestKasseListReturnsCashbookSheets(t *testing.T) {
	var method, path string
	var handlerErrors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/cashbooksheets":
			method = r.Method
			path = r.URL.Path
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"cashbookId": 2, "name": "Kasse"}}})
		default:
			handlerErrors = append(handlerErrors, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", postingConfigPath(t, server.URL), "kasse", "list"}); err != nil {
		t.Fatal(err)
	}
	if len(handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", handlerErrors)
	}
	if method != http.MethodPost || path != "/rest/cashbooksheets" {
		t.Fatalf("request = %s %s", method, path)
	}
	var records []map[string]any
	if err := json.Unmarshal(output.Bytes(), &records); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if len(records) != 1 || records[0]["cashbookId"] != float64(2) {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestKasseCreateDryRunDoesNotCallAPI(t *testing.T) {
	const fixture = `{"cashbookId":2,"documentDate":1788134400000,"debitAmount":18}`
	server, observation := newKasseServer(t, http.StatusCreated, "")
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", postingConfigPath(t, server.URL),
		"kasse", "create", "--data", fixture, "--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.requests != 0 {
		t.Fatalf("requests = %d", observation.requests)
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "dry_run" || status["endpoint"] != "POST /cashbookentry/new" {
		t.Fatalf("stdout = %s", output.String())
	}
	request, ok := status["request"].(map[string]any)
	if !ok || request["cashbookId"] != float64(2) || request["debitAmount"] != float64(18) {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestKasseCreateRejectsInvalidData(t *testing.T) {
	testCases := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "missing data", wantErr: "requires --data"},
		{name: "malformed JSON", data: `{`, wantErr: "JSON object"},
		{name: "array", data: `[]`, wantErr: "JSON object"},
		{name: "null", data: `null`, wantErr: "JSON object"},
		{name: "missing cashbook id", data: `{"documentDate":1788134400000}`, wantErr: "cashbookId is required"},
		{name: "non-integer cashbook id", data: `{"cashbookId":2.5,"documentDate":1788134400000}`, wantErr: "cashbookId must be an int64"},
		{name: "missing document date", data: `{"cashbookId":2}`, wantErr: "documentDate is required"},
		{name: "non-integer document date", data: `{"cashbookId":2,"documentDate":"2026-09-01"}`, wantErr: "documentDate must be an int64"},
		{name: "read-only document number", data: `{"cashbookId":2,"documentDate":1788134400000,"internalDocumentNumber":"K1000"}`, wantErr: "read-only"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, observation := newKasseServer(t, http.StatusCreated, "")
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			withCLI(t, "", false)
			args := []string{"--config", postingConfigPath(t, server.URL), "kasse", "create"}
			if testCase.data != "" {
				args = append(args, "--data", testCase.data)
			}

			err := run(args)
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, testCase.wantErr)
			}
			if observation.requests != 0 {
				t.Fatalf("requests = %d", observation.requests)
			}
		})
	}
}

func TestKasseCreateRequiresConfirmation(t *testing.T) {
	const fixture = `{"cashbookId":2,"documentDate":1788134400000,"creditAmount":23}`
	server, observation := newKasseServer(t, http.StatusCreated, `{"id":7001}`)
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	configPath := postingConfigPath(t, server.URL)

	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "kasse", "create", "--data", fixture})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("error = %v", err)
	}
	if observation.requests != 0 {
		t.Fatalf("requests before confirmation = %d", observation.requests)
	}

	output, _ := withCLI(t, "create kassenbuchung 2\n", true)
	err = run([]string{"--config", configPath, "kasse", "create", "--data", fixture})
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

func TestKasseCreatePreservesAPIErrors(t *testing.T) {
	testCases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "rejected", status: http.StatusBadRequest, body: "cashbook unavailable"},
		{name: "ambiguous", status: http.StatusInternalServerError, body: "temporary failure"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server, observation := newKasseServer(t, testCase.status, testCase.body)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			output, _ := withCLI(t, "", false)

			err := run([]string{
				"--config", postingConfigPath(t, server.URL),
				"kasse", "create",
				"--data", `{"cashbookId":2,"documentDate":1788134400000,"creditAmount":23}`,
				"--yes",
			})
			if err == nil || !strings.Contains(err.Error(), testCase.body) {
				t.Fatalf("error = %v", err)
			}
			if observation.mutations != 1 {
				t.Fatalf("mutations = %d", observation.mutations)
			}
			if strings.Contains(output.String(), `"status": "created"`) {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}
