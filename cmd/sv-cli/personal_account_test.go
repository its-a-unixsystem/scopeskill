package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type personalAccountStub struct {
	server             *httptest.Server
	accountBodies      map[string][]map[string]any
	accountRecords     map[string][]any
	bankConnections    map[string]any
	bankConnectionHits []string
	contactBodies      []map[string]any
	contactPages       [][]any
	contactIdx         int
	contactByID        map[string]map[string]any
	fiscalYearHits     int
	susaQueries        map[string][]url.Values
	susaRecords        map[string][]any
}

func newPersonalAccountStub(t *testing.T) *personalAccountStub {
	t.Helper()
	stub := &personalAccountStub{
		accountBodies:   map[string][]map[string]any{},
		accountRecords:  map[string][]any{},
		bankConnections: map[string]any{},
		contactByID:     map[string]map[string]any{},
		susaQueries:     map[string][]url.Values{},
		susaRecords:     map[string][]any{},
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case (r.URL.Path == "/rest/debitoraccounts" || r.URL.Path == "/rest/kreditoraccounts") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.accountBodies[r.URL.Path] = append(stub.accountBodies[r.URL.Path], body)
			records := stub.accountRecords[r.URL.Path]
			if records == nil {
				records = []any{}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case strings.HasPrefix(r.URL.Path, "/rest/debitoraccounts/") && strings.HasSuffix(r.URL.Path, "/bankConnections"):
			stub.bankConnectionHits = append(stub.bankConnectionHits, r.URL.Path)
			writeJSONForCLI(w, stub.bankConnections[r.URL.Path])
		case strings.HasPrefix(r.URL.Path, "/rest/kreditoraccounts/") && strings.HasSuffix(r.URL.Path, "/bankConnections"):
			stub.bankConnectionHits = append(stub.bankConnectionHits, r.URL.Path)
			writeJSONForCLI(w, stub.bankConnections[r.URL.Path])
		case r.URL.Path == "/rest/contacts" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.contactBodies = append(stub.contactBodies, body)
			page := []any{}
			if stub.contactIdx < len(stub.contactPages) {
				page = stub.contactPages[stub.contactIdx]
				stub.contactIdx++
			}
			writeJSONForCLI(w, map[string]any{"records": page})
		case strings.HasPrefix(r.URL.Path, "/rest/contact/"):
			id := strings.TrimPrefix(r.URL.Path, "/rest/contact/")
			rec, ok := stub.contactByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusBadRequest)
				return
			}
			writeJSONForCLI(w, rec)
		case r.URL.Path == "/rest/fiscalyears":
			stub.fiscalYearHits++
			writeJSONForCLI(w, map[string]any{
				"years": []any{
					map[string]any{"id": 56, "name": "Eröffnungsbilanz", "beginning": "2024-12-31T00:00:00.000Z+0100", "open": true},
					map[string]any{"id": 58, "name": "2026", "beginning": "2026-01-01T00:00:00.000Z+0100", "open": true},
				},
			})
		case r.URL.Path == "/rest/datasource/susa/debtors" || r.URL.Path == "/rest/datasource/susa/creditors":
			stub.susaQueries[r.URL.Path] = append(stub.susaQueries[r.URL.Path], r.URL.Query())
			records := stub.susaRecords[r.URL.Path]
			if records == nil {
				records = []any{}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestDebitorSearchByNumberPostsExpectedBody(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.accountRecords["/rest/debitoraccounts"] = []any{
		map[string]any{"number": "10000", "name": "Kunden A-Z", "contactId": 49191.0},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "debitor", "search", "--number=10000"}); err != nil {
		t.Fatal(err)
	}

	bodies := stub.accountBodies["/rest/debitoraccounts"]
	if len(bodies) != 1 {
		t.Fatalf("debitor account requests = %d", len(bodies))
	}
	search := bodies[0]["search"].([]any)
	cond := search[0].(map[string]any)
	if cond["field"] != "number" || cond["operator"] != "equals" || cond["value"] != "10000" {
		t.Fatalf("condition = %#v", cond)
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got[0].(map[string]any)["number"] != "10000" {
		t.Fatalf("records = %#v", got)
	}
}

func TestDebitorSearchByNameRoutesThroughKontakt(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.contactPages = [][]any{{
		map[string]any{"id": 49191.0, "lastname": "Kunden A-Z", "debitorNumber": "10000", "kreditorNumber": nil},
		map[string]any{"id": 49192.0, "lastname": "Kunden ohne Debitor", "debitorNumber": nil, "kreditorNumber": nil},
	}}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "debitor", "search", "--name=Kunden", "--number-prefix=10"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.contactBodies) != 1 {
		t.Fatalf("contact requests = %d", len(stub.contactBodies))
	}
	cond := stub.contactBodies[0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "lastname" || cond["operator"] != "contains" || cond["value"] != "Kunden" {
		t.Fatalf("condition = %#v", cond)
	}
	if len(stub.accountBodies["/rest/debitoraccounts"]) != 0 {
		t.Fatalf("name search should not hit debitoraccounts: %#v", stub.accountBodies)
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if len(got) != 1 {
		t.Fatalf("records = %#v", got)
	}
	rec := got[0].(map[string]any)
	if rec["number"] != "10000" || rec["kontakt"].(map[string]any)["lastname"] != "Kunden A-Z" {
		t.Fatalf("record = %#v", rec)
	}
}

func TestKreditorSearchUsesKreditorEndpointAndActiveFilter(t *testing.T) {
	stub := newPersonalAccountStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kreditor", "search", "--number-prefix=70", "--active"}); err != nil {
		t.Fatal(err)
	}

	bodies := stub.accountBodies["/rest/kreditoraccounts"]
	if len(bodies) != 1 {
		t.Fatalf("kreditor account requests = %d", len(bodies))
	}
	search := bodies[0]["search"].([]any)
	if len(search) != 2 {
		t.Fatalf("conditions = %#v", search)
	}
	first := search[0].(map[string]any)
	if first["field"] != "number" || first["operator"] != "startswith" || first["value"] != "70" {
		t.Fatalf("number-prefix condition = %#v", first)
	}
	second := search[1].(map[string]any)
	if second["field"] != "active" || second["operator"] != "equals" || second["value"] != true {
		t.Fatalf("active condition = %#v", second)
	}
}

func TestPersonalAccountBankConnectionsFetchesAccountEndpoint(t *testing.T) {
	cases := []struct {
		command string
		path    string
	}{
		{"debitor", "/rest/debitoraccounts/10000/bankConnections"},
		{"kreditor", "/rest/kreditoraccounts/70000/bankConnections"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			stub.bankConnections[tc.path] = map[string]any{
				"records": []any{
					map[string]any{"iban": "DE02120300000000202051"},
				},
			}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)
			number := "10000"
			if tc.command == "kreditor" {
				number = "70000"
			}

			if err := run([]string{"--config", configPath, tc.command, "bank-connections", number}); err != nil {
				t.Fatal(err)
			}

			if len(stub.bankConnectionHits) != 1 || stub.bankConnectionHits[0] != tc.path {
				t.Fatalf("bank connection hits = %#v", stub.bankConnectionHits)
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			records := got["records"].([]any)
			if records[0].(map[string]any)["iban"] != "DE02120300000000202051" {
				t.Fatalf("records = %#v", records)
			}
		})
	}
}

func TestDebitorShowReturnsStitchedAccountKontaktAndSaldo(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.accountRecords["/rest/debitoraccounts"] = []any{
		map[string]any{"number": "10000", "name": "Kunden A-Z", "contactId": 49191.0},
	}
	stub.contactByID["49191"] = map[string]any{"id": 49191.0, "lastname": "Kunden A-Z", "debitorNumber": "10000"}
	stub.susaRecords["/rest/datasource/susa/debtors"] = []any{
		map[string]any{"Kontonummer": "10000", "Saldo": "42,00"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "debitor", "show", "10000"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["debitor"].(map[string]any)["number"] != "10000" {
		t.Fatalf("debitor = %#v", got["debitor"])
	}
	if got["kontakt"].(map[string]any)["lastname"] != "Kunden A-Z" {
		t.Fatalf("kontakt = %#v", got["kontakt"])
	}
	saldo := got["saldo"].(map[string]any)
	if saldo["current"].(map[string]any)["Saldo"] != "42,00" || saldo["fiscalYearToDate"].(map[string]any)["Saldo"] != "42,00" {
		t.Fatalf("saldo = %#v", saldo)
	}
	if len(stub.susaQueries["/rest/datasource/susa/debtors"]) != 2 {
		t.Fatalf("susa calls = %#v", stub.susaQueries)
	}
}

func TestKreditorShowReturnsNullKontaktWhenUnlinked(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.accountRecords["/rest/kreditoraccounts"] = []any{
		map[string]any{"number": "70000", "name": "Lieferanten A-Z", "contactId": nil},
	}
	stub.susaRecords["/rest/datasource/susa/creditors"] = []any{
		map[string]any{"Kontonummer": "70000", "Saldo": "-12,00"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kreditor", "show", "70000"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["kreditor"].(map[string]any)["number"] != "70000" {
		t.Fatalf("kreditor = %#v", got["kreditor"])
	}
	if _, ok := got["kontakt"]; !ok {
		t.Fatalf("kontakt key missing: %s", output.String())
	}
	if got["kontakt"] != nil {
		t.Fatalf("kontakt = %#v", got["kontakt"])
	}
}

func TestKreditorBalanceWithExplicitDatesUsesCreditorSusa(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.susaRecords["/rest/datasource/susa/creditors"] = []any{
		map[string]any{"Kontonummer": "70000", "Saldo": "-12,00"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kreditor", "balance", "70000", "--from=2026-01-01", "--to=2026-05-07"}); err != nil {
		t.Fatal(err)
	}

	queries := stub.susaQueries["/rest/datasource/susa/creditors"]
	if len(queries) != 1 {
		t.Fatalf("susa calls = %#v", stub.susaQueries)
	}
	if queries[0].Get("startDate") != "01.01.2026" || queries[0].Get("endDate") != "07.05.2026" {
		t.Fatalf("query = %v", queries[0])
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["Kontonummer"] != "70000" {
		t.Fatalf("got = %#v", got)
	}
}
