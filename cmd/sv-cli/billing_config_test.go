package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type billingConfigStub struct {
	server *httptest.Server
	hits   []string
}

func newBillingConfigStub(t *testing.T) *billingConfigStub {
	t.Helper()
	stub := &billingConfigStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/paymentterms":
			stub.hits = append(stub.hits, r.URL.Path)
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"id": 12, "name": "14 Tage netto"}}})
		case "/rest/paymentterm/12":
			stub.hits = append(stub.hits, r.URL.Path)
			writeJSONForCLI(w, map[string]any{"id": 12, "name": "14 Tage netto"})
		case "/rest/vatmatrixentries":
			stub.hits = append(stub.hits, r.URL.Path)
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"id": 7, "vatKey": "V19"}}})
		case "/rest/vatscopes":
			stub.hits = append(stub.hits, r.URL.Path)
			writeJSONForCLI(w, map[string]any{"records": []any{map[string]any{"id": 3, "name": "Inland"}}})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestBillingConfigCommandsFetchExpectedEndpoints(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantHit string
		want    string
	}{
		{"payment terms list", []string{"zahlungsbedingung", "list"}, "/rest/paymentterms", "14 Tage netto"},
		{"payment term show", []string{"zahlungsbedingung", "show", "12"}, "/rest/paymentterm/12", "14 Tage netto"},
		{"vat matrix list", []string{"steuermatrix", "list"}, "/rest/vatmatrixentries", "V19"},
		{"vat scopes list", []string{"steuersachverhalt", "list"}, "/rest/vatscopes", "Inland"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newBillingConfigStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			args := append([]string{"--config", configPath}, tc.args...)
			if err := run(args); err != nil {
				t.Fatal(err)
			}

			if len(stub.hits) != 1 || stub.hits[0] != tc.wantHit {
				t.Fatalf("hits = %#v", stub.hits)
			}
			var got any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			if !strings.Contains(output.String(), tc.want) {
				t.Fatalf("stdout missing %q: %s", tc.want, output.String())
			}
		})
	}
}
