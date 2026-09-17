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

func TestOffenePostenRebookRoutesBySide(t *testing.T) {
	testCases := []struct {
		name     string
		seite    string
		endpoint string
	}{
		{name: "Debitor", seite: "debitor", endpoint: "/rest/openitems/debitor/rebook"},
		{name: "Kreditor", seite: "kreditor", endpoint: "/rest/openitems/creditor/rebook"},
	}
	request := map[string]any{
		"documentNumber": "2025-000012",
		"postingDate":    "31.12.2025",
		"postingPeriod":  "13",
		"sourceNumber":   "10002",
		"targetNumber":   "10001",
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var mutations int
			var gotBody map[string]any
			var handlerErrors []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/token":
					writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
				case testCase.endpoint:
					mutations++
					if r.Method != http.MethodPost {
						handlerErrors = append(handlerErrors, "method = "+r.Method)
					}
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						handlerErrors = append(handlerErrors, err.Error())
						http.Error(w, "request body unavailable", http.StatusInternalServerError)
						return
					}
					if err := json.Unmarshal(raw, &gotBody); err != nil {
						handlerErrors = append(handlerErrors, err.Error())
						http.Error(w, "invalid JSON", http.StatusBadRequest)
						return
					}
					writeJSONForCLI(w, map[string]any{"success": true})
				default:
					handlerErrors = append(handlerErrors, r.Method+" "+r.URL.Path)
					http.Error(w, "unexpected request", http.StatusInternalServerError)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			output, _ := withCLI(t, "", false)
			rawRequest, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}

			err = run([]string{
				"--config", postingConfigPath(t, server.URL),
				"offene-posten", "rebook", "--seite=" + testCase.seite,
				"--data", string(rawRequest), "--yes",
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(handlerErrors) != 0 {
				t.Fatalf("handler errors = %v", handlerErrors)
			}
			if mutations != 1 {
				t.Fatalf("mutations = %d", mutations)
			}
			if !reflect.DeepEqual(gotBody, request) {
				t.Fatalf("request body = %#v", gotBody)
			}
			status := stdoutStatus(t, output.String())
			if status["status"] != "rebooked" || status["endpoint"] != "POST "+testCase.endpoint[5:] {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}

func TestOffenePostenSetReminderLevelSurfacesPerItemFailures(t *testing.T) {
	request := map[string]any{
		"openItems": []any{
			map[string]any{"documentNumber": "2026-100", "reminderLevel": float64(2)},
			map[string]any{"documentNumber": "2026-404", "reminderLevel": float64(3)},
		},
	}
	var mutations int
	var gotBody map[string]any
	var handlerErrors []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/openitems/setReminderLevel":
			mutations++
			if r.Method != http.MethodPost {
				handlerErrors = append(handlerErrors, "method = "+r.Method)
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				handlerErrors = append(handlerErrors, err.Error())
				http.Error(w, "request body unavailable", http.StatusInternalServerError)
				return
			}
			if err := json.Unmarshal(raw, &gotBody); err != nil {
				handlerErrors = append(handlerErrors, err.Error())
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			writeJSONForCLI(w, map[string]any{
				"openItems": []any{
					map[string]any{"success": true, "documentNumber": "2026-100", "reminderLevel": 2},
					map[string]any{"success": false, "documentNumber": "2026-404", "message": "open item not found"},
				},
			})
		default:
			handlerErrors = append(handlerErrors, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)
	rawRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	err = run([]string{
		"--config", postingConfigPath(t, server.URL),
		"offene-posten", "set-reminder-level",
		"--data", string(rawRequest), "--yes",
	})
	if err == nil || !strings.Contains(err.Error(), "2026-404") || !strings.Contains(err.Error(), "open item not found") {
		t.Fatalf("error = %v", err)
	}
	if len(handlerErrors) != 0 {
		t.Fatalf("handler errors = %v", handlerErrors)
	}
	if mutations != 1 {
		t.Fatalf("mutations = %d", mutations)
	}
	if !reflect.DeepEqual(gotBody, request) {
		t.Fatalf("request body = %#v", gotBody)
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "partial_failure" || status["endpoint"] != "POST /openitems/setReminderLevel" {
		t.Fatalf("stdout = %s", output.String())
	}
	failures, ok := status["failures"].([]any)
	if !ok || len(failures) != 1 || failures[0].(map[string]any)["documentNumber"] != "2026-404" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestOffenePostenSetReminderLevelReportsSuccess(t *testing.T) {
	var mutations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
		case "/rest/openitems/setReminderLevel":
			mutations++
			writeJSONForCLI(w, map[string]any{
				"openItems": []any{
					map[string]any{"success": true, "documentNumber": "2026-100", "reminderLevel": 2},
				},
			})
		default:
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv(scopeskill.EnvBaseURL, "")
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", postingConfigPath(t, server.URL),
		"offene-posten", "set-reminder-level",
		"--data", `{"openItems":[{"documentNumber":"2026-100","reminderLevel":2}]}`, "--yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutations != 1 {
		t.Fatalf("mutations = %d", mutations)
	}
	if status := stdoutStatus(t, output.String()); status["status"] != "updated" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestOffenePostenMaintenanceWriteSafety(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "rebook",
			args: []string{
				"offene-posten", "rebook", "--seite=debitor",
				"--data", `{"documentNumber":"2025-000012","sourceNumber":"10002","targetNumber":"10001"}`,
			},
		},
		{
			name: "set reminder level",
			args: []string{
				"offene-posten", "set-reminder-level",
				"--data", `{"openItems":[{"documentNumber":"2026-100","reminderLevel":2}]}`,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			configPath := postingConfigPath(t, server.URL)

			t.Run("dry run", func(t *testing.T) {
				output, _ := withCLI(t, "", false)
				args := append([]string{"--config", configPath}, testCase.args...)
				args = append(args, "--dry-run")
				if err := run(args); err != nil {
					t.Fatal(err)
				}
				if status := stdoutStatus(t, output.String()); status["status"] != "dry_run" {
					t.Fatalf("stdout = %s", output.String())
				}
			})
			t.Run("confirmation", func(t *testing.T) {
				withCLI(t, "", false)
				args := append([]string{"--config", configPath}, testCase.args...)
				err := run(args)
				if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
					t.Fatalf("error = %v", err)
				}
			})
			if requests != 0 {
				t.Fatalf("requests = %d", requests)
			}
		})
	}
}

func TestOffenePostenMaintenancePreservesAPIErrors(t *testing.T) {
	testCases := []struct {
		name       string
		endpoint   string
		statusCode int
		response   string
		args       []string
	}{
		{
			name:       "rebook rejection",
			endpoint:   "/rest/openitems/creditor/rebook",
			statusCode: http.StatusBadRequest,
			response:   "source account is invalid",
			args: []string{
				"offene-posten", "rebook", "--seite=kreditor",
				"--data", `{"documentNumber":"2025-000012","sourceNumber":"10002","targetNumber":"10001"}`,
			},
		},
		{
			name:       "reminder server failure",
			endpoint:   "/rest/openitems/setReminderLevel",
			statusCode: http.StatusInternalServerError,
			response:   "temporary reminder failure",
			args: []string{
				"offene-posten", "set-reminder-level",
				"--data", `{"openItems":[{"documentNumber":"2026-100","reminderLevel":2}]}`,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var mutations int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/token":
					writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "access", "expires_in": 3600})
				case testCase.endpoint:
					mutations++
					http.Error(w, testCase.response, testCase.statusCode)
				default:
					http.Error(w, "unexpected request", http.StatusInternalServerError)
				}
			}))
			t.Cleanup(server.Close)
			t.Setenv(scopeskill.EnvBaseURL, "")
			t.Setenv(scopeskill.EnvRestRefreshToken, "")
			output, _ := withCLI(t, "", false)
			args := append([]string{"--config", postingConfigPath(t, server.URL)}, testCase.args...)
			args = append(args, "--yes")

			err := run(args)
			if err == nil || !strings.Contains(err.Error(), testCase.response) {
				t.Fatalf("error = %v", err)
			}
			if mutations != 1 {
				t.Fatalf("mutations = %d", mutations)
			}
			if output.Len() != 0 {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}
