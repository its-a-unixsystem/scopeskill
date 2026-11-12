package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type personenkontoJournalStub struct {
	server   *httptest.Server
	requests []searchEndpointRequest
	pages    map[string][][]any
}

type searchEndpointRequest struct {
	method string
	path   string
	body   map[string]any
}

func newPersonenkontoJournalStub(t *testing.T) *personenkontoJournalStub {
	t.Helper()
	stub := &personenkontoJournalStub{pages: map[string][][]any{}}
	pageIndex := map[string]int{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/personalkonto":
			http.Error(w, "wrong path", http.StatusInternalServerError)
		default:
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method for %s: %s", r.URL.Path, r.Method)
				http.Error(w, "unexpected method", http.StatusInternalServerError)
				return
			}
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.requests = append(stub.requests, searchEndpointRequest{method: r.Method, path: r.URL.Path, body: body})
			pages := stub.pages[r.URL.Path]
			if after, ok := body["afterId"].(float64); r.URL.Path == "/rest/personaljournal" && ok {
				if body["order"] != nil || body["page"] != float64(0) {
					t.Errorf("invalid cursor request: %#v", body)
				}
				records := []any{}
				for _, page := range pages {
					for _, item := range page {
						row := item.(map[string]any)
						if row["rowNumber"].(int) > int(after) && len(records) < int(body["pageSize"].(float64)) {
							records = append(records, item)
						}
					}
				}
				writeJSONForCLI(w, map[string]any{"records": records})
				return
			}
			idx := pageIndex[r.URL.Path]
			if idx >= len(pages) {
				t.Errorf("unexpected page for %s", r.URL.Path)
				http.Error(w, "unexpected page", http.StatusInternalServerError)
				return
			}
			pageIndex[r.URL.Path] = idx + 1
			writeJSONForCLI(w, map[string]any{"records": pages[idx]})
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestPersonenkontoJournalCommandsUseExpectedEndpoints(t *testing.T) {
	cases := []struct {
		name string
		args []string
		path string
	}{
		{"personenkonto", []string{"personenkonto", "journal", "--page-size=25"}, "/rest/personaljournal"},
		{"debitor", []string{"debitor", "journal", "--page-size=25"}, "/rest/openitems/debitor/list"},
		{"kreditor", []string{"kreditor", "journal", "--page-size=25"}, "/rest/openitems/creditor/list"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newPersonenkontoJournalStub(t)
			stub.pages[tc.path] = [][]any{{map[string]any{"documentNumber": tc.name}}}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			args := append([]string{"--config", configPath}, tc.args...)
			if err := run(args); err != nil {
				t.Fatal(err)
			}

			if len(stub.requests) != 1 {
				t.Fatalf("requests = %#v", stub.requests)
			}
			req := stub.requests[0]
			if req.path != tc.path {
				t.Fatalf("path = %s, want %s", req.path, tc.path)
			}
			if req.body["page"].(float64) != 0 || req.body["pageSize"].(float64) != 25 {
				t.Fatalf("body = %#v", req.body)
			}
			if !strings.Contains(output.String(), tc.name) {
				t.Fatalf("stdout = %s", output.String())
			}
		})
	}
}

func TestPersonenkontoJournalAllPaginates(t *testing.T) {
	full := make([]any, scopeskill.MaxSearchPageSize)
	for i := range full {
		full[i] = map[string]any{"rowNumber": i + 1, "documentNumber": "same-document"}
	}
	stub := newPersonenkontoJournalStub(t)
	stub.pages["/rest/personaljournal"] = [][]any{
		full,
		{map[string]any{"rowNumber": 1001, "documentNumber": "same-document"}},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "personenkonto", "journal", "--all"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.requests) != 2 {
		t.Fatalf("requests = %d", len(stub.requests))
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if len(got) != scopeskill.MaxSearchPageSize+1 {
		t.Fatalf("len = %d", len(got))
	}
	for i, item := range got {
		if item.(map[string]any)["rowNumber"] != float64(i+1) {
			t.Fatalf("row %d = %#v", i+1, item)
		}
	}
}
