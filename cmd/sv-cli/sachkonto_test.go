package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

// fixedNow installs a deterministic nowFunc for the test and reverts on
// cleanup.
func fixedNow(t *testing.T, ts time.Time) {
	t.Helper()
	old := nowFunc
	nowFunc = func() time.Time { return ts }
	t.Cleanup(func() { nowFunc = old })
}

type sachkontoStub struct {
	server   *httptest.Server
	requests []sachkontoRequest
}

type sachkontoRequest struct {
	body map[string]any
}

func newSachkontoStub(t *testing.T, pages [][]any) *sachkontoStub {
	t.Helper()
	stub := &sachkontoStub{}
	idx := 0
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/impersonalaccounts":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("body parse: %v raw=%s", err, raw)
			}
			stub.requests = append(stub.requests, sachkontoRequest{body: body})
			if idx >= len(pages) {
				t.Fatalf("unexpected extra /impersonalaccounts call %d", idx)
			}
			writeJSONForCLI(w, pages[idx])
			idx++
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func sachkontoConfigPath(t *testing.T, baseURL string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + baseURL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return configPath
}

func TestSachkontoSearchByNamePostsExpectedBodyAndReturnsRecords(t *testing.T) {
	page := []any{
		map[string]any{"id": 1.0, "number": "4400", "name": "Reisekosten"},
	}
	stub := newSachkontoStub(t, [][]any{page})
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "search", "--name=Reisekosten"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.requests) != 1 {
		t.Fatalf("requests = %d", len(stub.requests))
	}
	body := stub.requests[0].body
	if body["pageSize"].(float64) != 100 {
		t.Fatalf("pageSize = %v", body["pageSize"])
	}
	if body["page"].(float64) != 0 {
		t.Fatalf("page = %v", body["page"])
	}
	search := body["search"].([]any)
	if len(search) != 1 {
		t.Fatalf("search = %#v", search)
	}
	cond := search[0].(map[string]any)
	if cond["field"] != "name" || cond["operator"] != "contains" || cond["value"] != "Reisekosten" {
		t.Fatalf("condition = %#v", cond)
	}

	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON parse: %v: %s", err, output.String())
	}
	if len(got) != 1 {
		t.Fatalf("records = %#v", got)
	}
	rec := got[0].(map[string]any)
	if rec["number"] != "4400" {
		t.Fatalf("record = %#v", rec)
	}
}

func TestSachkontoSearchAllUsesPageSize1000AndStopsAtSafetyCap(t *testing.T) {
	full := make([]any, scopeskill.MaxSearchPageSize)
	for i := range full {
		full[i] = map[string]any{"id": float64(i), "number": fmt.Sprintf("4%03d", i)}
	}
	pages := make([][]any, 12)
	for i := range pages {
		pages[i] = full
	}
	stub := newSachkontoStub(t, pages)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "search", "--number-prefix=4", "--all"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.requests) != 10 {
		t.Fatalf("requests = %d (expected 10 full pages, then stop)", len(stub.requests))
	}
	for i, req := range stub.requests {
		if req.body["pageSize"].(float64) != float64(scopeskill.MaxSearchPageSize) {
			t.Fatalf("call %d pageSize = %v", i, req.body["pageSize"])
		}
		if req.body["page"].(float64) != float64(i) {
			t.Fatalf("call %d page = %v", i, req.body["page"])
		}
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON parse: %v", err)
	}
	if len(got) != scopeskill.DefaultMaxResults {
		t.Fatalf("len = %d, want %d", len(got), scopeskill.DefaultMaxResults)
	}
}

func TestSachkontoSearchDataWithAllIsRejected(t *testing.T) {
	stub := newSachkontoStub(t, nil)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	dataPath := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(dataPath, []byte(`{"page":0,"pageSize":50}`), 0o600); err != nil {
		t.Fatal(err)
	}
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "sachkonto", "search", "--data", "@" + dataPath, "--all"})
	if err == nil || !strings.Contains(err.Error(), "--data cannot be combined") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(stub.requests))
	}
}

func TestSachkontoSearchDataWithPageSizeIsRejected(t *testing.T) {
	stub := newSachkontoStub(t, nil)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "sachkonto", "search", "--data", `{"page":0,"pageSize":1}`, "--page-size=200"})
	if err == nil || !strings.Contains(err.Error(), "--data cannot be combined") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.requests) != 0 {
		t.Fatalf("requests = %d, want 0", len(stub.requests))
	}
}

func TestSachkontoSearchDataOverridePostsRawBody(t *testing.T) {
	page := []any{map[string]any{"id": 9.0, "number": "0001"}}
	stub := newSachkontoStub(t, [][]any{page})
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "search", "--data", `{"page":3,"pageSize":7,"fields":["custom"]}`}); err != nil {
		t.Fatal(err)
	}
	if len(stub.requests) != 1 {
		t.Fatalf("requests = %d", len(stub.requests))
	}
	body := stub.requests[0].body
	if body["page"].(float64) != 3 || body["pageSize"].(float64) != 7 {
		t.Fatalf("body = %#v", body)
	}
	if !strings.Contains(output.String(), `"number": "0001"`) {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestSachkontoSearchPropagatesAPIErrorToStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		default:
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	configPath := sachkontoConfigPath(t, server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "sachkonto", "search", "--name=x"})
	if err == nil {
		t.Fatal("expected error from 5xx response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v", err)
	}
}

func TestSachkontoSearchHelpDocumentsLimits(t *testing.T) {
	_, stderr := withCLI(t, "", false)
	err := run([]string{"sachkonto", "search", "--help"})
	if err == nil {
		t.Fatal("flag.ContinueOnError should surface --help as an error")
	}
	for _, want := range []string{"pageSize=100", "10000", "--max", "--all", "--data"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("help missing %q in %q", want, stderr.String())
		}
	}
}

func TestSachkontoWithoutSubcommandPrintsHelpAndFails(t *testing.T) {
	output, _ := withCLI(t, "", false)
	client := scopeskill.NewClient(scopeskill.Config{AccessToken: "access-token"})
	err := sachkonto(client, nil)
	if err == nil {
		t.Fatal("expected missing sachkonto subcommand error")
	}
	for _, want := range []string{"search", "show", "balance"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q (missing %q)", output.String(), want)
		}
	}
}

// showStub canned a Mandant with two fiscal years (2025, 2026) and a single
// Sachkonto 4400. Used by show + balance tests.
type showStub struct {
	server         *httptest.Server
	searchBodies   []map[string]any
	susaQueries    []url.Values
	fiscalYearHits int
	kontoRecords   []any
	susaResponse   func(query url.Values) any
}

func newShowStub(t *testing.T) *showStub {
	t.Helper()
	stub := &showStub{
		kontoRecords: []any{
			map[string]any{"id": 1.0, "number": "4400", "name": "Erlöse 19% / 16% USt", "active": true, "accountTypeName": "Erlöse"},
		},
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case "/rest/impersonalaccounts":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.searchBodies = append(stub.searchBodies, body)
			writeJSONForCLI(w, stub.kontoRecords)
		case "/rest/fiscalyears":
			stub.fiscalYearHits++
			writeJSONForCLI(w, map[string]any{
				"years": []any{
					map[string]any{"id": 56, "name": "Eröffnungsbilanz", "beginning": "2024-12-31T00:00:00.000Z+0100", "open": true},
					map[string]any{"id": 45, "name": "2025", "beginning": "2025-01-01T00:00:00.000Z+0100", "open": true},
					map[string]any{"id": 58, "name": "2026", "beginning": "2026-01-01T00:00:00.000Z+0100", "open": true},
				},
			})
		case "/rest/datasource/susa/impersonalAccounts":
			stub.susaQueries = append(stub.susaQueries, r.URL.Query())
			if stub.susaResponse != nil {
				writeJSONForCLI(w, stub.susaResponse(r.URL.Query()))
				return
			}
			writeJSONForCLI(w, map[string]any{"records": []any{
				map[string]any{
					"Kontonummer":     "4400",
					"Kontoname":       "Erlöse 19% / 16% USt",
					"Saldenvortrag":   "0,00",
					"Saldo":           "42,00",
					"Saldo-Kumuliert": "42,00",
					"Soll":            "0,00",
					"Haben":           "42,00",
				},
			}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestSachkontoShowReturnsStitchedKontoAndSaldo(t *testing.T) {
	stub := newShowStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "show", "4400"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	konto := got["konto"].(map[string]any)
	if konto["number"] != "4400" {
		t.Fatalf("konto = %#v", konto)
	}
	saldo := got["saldo"].(map[string]any)
	if _, ok := saldo["current"]; !ok {
		t.Fatalf("saldo missing current: %#v", saldo)
	}
	if _, ok := saldo["fiscalYearToDate"]; !ok {
		t.Fatalf("saldo missing fiscalYearToDate: %#v", saldo)
	}
	if stub.fiscalYearHits != 1 {
		t.Fatalf("fiscalyears hits = %d", stub.fiscalYearHits)
	}
	if len(stub.susaQueries) != 2 {
		t.Fatalf("expected 2 SuSa calls, got %d", len(stub.susaQueries))
	}
	if stub.susaQueries[0].Get("startDate") != "31.12.2024" || stub.susaQueries[0].Get("endDate") != "07.05.2026" {
		t.Fatalf("current susa query = %v", stub.susaQueries[0])
	}
	if stub.susaQueries[1].Get("startDate") != "01.01.2026" || stub.susaQueries[1].Get("endDate") != "07.05.2026" {
		t.Fatalf("fyToDate susa query = %v", stub.susaQueries[1])
	}
	search := stub.searchBodies[0]["search"].([]any)
	cond := search[0].(map[string]any)
	if cond["field"] != "number" || cond["operator"] != "equals" || cond["value"] != "4400" {
		t.Fatalf("konto search condition = %#v", cond)
	}
}

func TestSachkontoShowReturnsNotFoundForUnknownNumber(t *testing.T) {
	stub := newShowStub(t)
	stub.kontoRecords = []any{}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "sachkonto", "show", "9999"})
	if err == nil || !strings.Contains(err.Error(), "not found or authorization missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestSachkontoBalanceWithExplicitDates(t *testing.T) {
	stub := newShowStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "balance", "4400", "--from=2025-01-01", "--to=2025-12-31"}); err != nil {
		t.Fatal(err)
	}

	if stub.fiscalYearHits != 0 {
		t.Fatalf("fiscalyears should not be queried with explicit dates: hits = %d", stub.fiscalYearHits)
	}
	if len(stub.susaQueries) != 1 {
		t.Fatalf("expected 1 SuSa call, got %d", len(stub.susaQueries))
	}
	if stub.susaQueries[0].Get("startDate") != "01.01.2025" || stub.susaQueries[0].Get("endDate") != "31.12.2025" {
		t.Fatalf("susa query = %v", stub.susaQueries[0])
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["Kontonummer"] != "4400" {
		t.Fatalf("got = %#v", got)
	}
}

func TestSachkontoBalanceDefaultsToCurrentFiscalYear(t *testing.T) {
	stub := newShowStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "sachkonto", "balance", "4400"}); err != nil {
		t.Fatal(err)
	}

	if stub.fiscalYearHits != 1 {
		t.Fatalf("fiscalyears hits = %d", stub.fiscalYearHits)
	}
	if len(stub.susaQueries) != 1 {
		t.Fatalf("susa calls = %d", len(stub.susaQueries))
	}
	if stub.susaQueries[0].Get("startDate") != "01.01.2026" || stub.susaQueries[0].Get("endDate") != "07.05.2026" {
		t.Fatalf("susa query = %v", stub.susaQueries[0])
	}
}

func TestSachkontoBalanceReturnsNotFoundWhenAccountAbsent(t *testing.T) {
	stub := newShowStub(t)
	stub.susaResponse = func(_ url.Values) any {
		return map[string]any{"records": []any{}}
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "sachkonto", "balance", "4400"})
	if err == nil || !strings.Contains(err.Error(), "not found or authorization missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestSachkontoBalanceRejectsInvalidDateFormat(t *testing.T) {
	stub := newShowStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "sachkonto", "balance", "4400", "--from=01.01.2025"})
	if err == nil || !strings.Contains(err.Error(), "--from") {
		t.Fatalf("err = %v", err)
	}
}
