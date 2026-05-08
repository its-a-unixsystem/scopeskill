package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type dimensionStub struct {
	server *httptest.Server
	hits   []string
	bodies []map[string]any
}

func newDimensionStub(t *testing.T) *dimensionStub {
	t.Helper()
	stub := &dimensionStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/dimensions":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.bodies = append(stub.bodies, body)
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"name": "Kostenstellen"}}})
		case "/rest/dimensions/Kostenstellen/dimensionentries":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"number": "K100"}}})
		case "/rest/texttemplates":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"name": "Mahnung"}}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestDimensionAndTextbausteinCommandsFetchExpectedEndpoints(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"dimension search", []string{"dimension", "search", "--page-size=20"}, "POST /rest/dimensions"},
		{"dimension entries", []string{"dimension", "entries", "Kostenstellen", "--page=2", "--page-size=50"}, "GET /rest/dimensions/Kostenstellen/dimensionentries?page=2&pageSize=50"},
		{"text templates", []string{"textbaustein", "list"}, "GET /rest/texttemplates"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newDimensionStub(t)
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
