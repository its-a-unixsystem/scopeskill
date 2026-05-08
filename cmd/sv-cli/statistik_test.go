package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type statistikStub struct {
	server *httptest.Server
	hits   []string
	bodies []map[string]any
}

func newStatistikStub(t *testing.T) *statistikStub {
	t.Helper()
	stub := &statistikStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/statisticsaccounts", "/rest/statisticspostings":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.bodies = append(stub.bodies, body)
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"number": "9000", "rowNumber": 44}}})
		case "/rest/statisticsaccount/9000", "/rest/statisticspostings/44":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			writeJSONForCLI(w, map[string]any{"number": "9000", "rowNumber": 44})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestStatistikCommandsFetchExpectedEndpoints(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"konto search", []string{"statistik", "konto", "search", "--page-size=15"}, "POST /rest/statisticsaccounts"},
		{"konto show", []string{"statistik", "konto", "show", "9000"}, "GET /rest/statisticsaccount/9000"},
		{"buchung search", []string{"statistik", "buchung", "search", "--page-size=15"}, "POST /rest/statisticspostings"},
		{"buchung show", []string{"statistik", "buchung", "show", "44"}, "GET /rest/statisticspostings/44"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStatistikStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			args := append([]string{"--config", configPath}, tc.args...)
			if err := run(args); err != nil {
				t.Fatal(err)
			}

			if len(stub.hits) != 1 || stub.hits[0] != tc.want {
				t.Fatalf("hits = %#v, want %q", stub.hits, tc.want)
			}
			var got any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
		})
	}
}
