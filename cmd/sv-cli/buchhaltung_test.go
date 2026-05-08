package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type buchhaltungStub struct {
	server *httptest.Server
	hits   []string
}

func newBuchhaltungStub(t *testing.T) *buchhaltungStub {
	t.Helper()
	stub := &buchhaltungStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/accountinginfo", "/rest/accountmapping", "/rest/gainandlossadjustmentaccounts":
			stub.hits = append(stub.hits, r.Method+" "+r.URL.RequestURI())
			writeJSONForCLI(w, map[string]any{"endpoint": r.URL.Path})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestBuchhaltungCommandsFetchExpectedEndpoints(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"info", []string{"buchhaltung", "info"}, "GET /rest/accountinginfo"},
		{"mapping", []string{"buchhaltung", "mapping"}, "GET /rest/accountmapping"},
		{"gewinn und verlust", []string{"buchhaltung", "gewinn-und-verlust"}, "GET /rest/gainandlossadjustmentaccounts"},
		{"gewinn und verlust with date", []string{"buchhaltung", "gewinn-und-verlust", "--balance-date=31.12.2025"}, "GET /rest/gainandlossadjustmentaccounts?balanceDate=31.12.2025"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newBuchhaltungStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			args := append([]string{"--config", configPath}, tc.args...)
			if err := run(args); err != nil {
				t.Fatal(err)
			}

			if len(stub.hits) != 1 || stub.hits[0] != tc.want {
				t.Fatalf("hits = %#v, want %q", stub.hits, tc.want)
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
		})
	}
}
