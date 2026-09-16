package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type personalAccountStub struct {
	server             *httptest.Server
	accountBodies      map[string][]map[string]any
	accountRecords     map[string][]any
	createBodies       map[string][]map[string]any
	updateBodies       map[string][]map[string]any
	createResponses    map[string]map[string]any
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
		createBodies:    map[string][]map[string]any{},
		updateBodies:    map[string][]map[string]any{},
		createResponses: map[string]map[string]any{},
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
		case (strings.HasPrefix(r.URL.Path, "/rest/debitoraccounts/") || strings.HasPrefix(r.URL.Path, "/rest/kreditoraccounts/")) && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.updateBodies[r.URL.Path] = append(stub.updateBodies[r.URL.Path], body)
			writeJSONForCLI(w, map[string]any{"status": "ok"})
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
		case (r.URL.Path == "/rest/createdebitor" || r.URL.Path == "/rest/createkreditor") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.createBodies[r.URL.Path] = append(stub.createBodies[r.URL.Path], body)
			response := stub.createResponses[r.URL.Path]
			if contactID := nonEmptyString(body["contactId"]); contactID != "" {
				if contact := stub.contactByID[contactID]; contact != nil {
					field := "debitorNumber"
					if r.URL.Path == "/rest/createkreditor" {
						field = "kreditorNumber"
					}
					contact[field] = response["number"]
				}
			}
			writeJSONForCLI(w, response)
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

func TestPersonalAccountShowKeepsNullSaldoForInactiveAccount(t *testing.T) {
	cases := []struct {
		command     string
		number      string
		accountPath string
	}{
		{"debitor", "10000", "/rest/debitoraccounts"},
		{"kreditor", "70000", "/rest/kreditoraccounts"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			stub.accountRecords[tc.accountPath] = []any{
				map[string]any{"number": tc.number, "name": "Inactive", "contactId": nil},
			}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			fixedNow(t, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
			output, _ := withCLI(t, "", false)

			if err := run([]string{"--config", configPath, tc.command, "show", tc.number}); err != nil {
				t.Fatal(err)
			}

			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			saldo := got["saldo"].(map[string]any)
			if saldo["current"] != nil || saldo["fiscalYearToDate"] != nil {
				t.Fatalf("saldo = %#v", saldo)
			}
		})
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
	if len(stub.accountBodies["/rest/kreditoraccounts"]) != 0 {
		t.Fatalf("active Saldo should not query master data: %#v", stub.accountBodies)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["Kontonummer"] != "70000" {
		t.Fatalf("got = %#v", got)
	}
}

func TestPersonalAccountBalanceSynthesizesZeroSaldoForInactiveAccount(t *testing.T) {
	cases := []struct {
		command     string
		number      string
		name        string
		accountPath string
		susaPath    string
	}{
		{"debitor", "10000", "Kunden A-Z", "/rest/debitoraccounts", "/rest/datasource/susa/debtors"},
		{"kreditor", "70000", "Lieferanten A-Z", "/rest/kreditoraccounts", "/rest/datasource/susa/creditors"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			stub.accountRecords[tc.accountPath] = []any{
				map[string]any{"number": tc.number, "name": tc.name},
			}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			if err := run([]string{"--config", configPath, tc.command, "balance", tc.number, "--from=2025-01-01", "--to=2025-12-31"}); err != nil {
				t.Fatal(err)
			}

			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			want := map[string]any{
				"Kontonummer":     tc.number,
				"Kontoname":       tc.name,
				"Haben":           "0,00",
				"Haben-Kumuliert": "0,00",
				"Saldenvortrag":   "0,00",
				"Saldo":           "0,00",
				"Saldo-Kumuliert": "0,00",
				"Soll":            "0,00",
				"Soll-Kumuliert":  "0,00",
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("saldo = %#v, want %#v", got, want)
			}
			if len(stub.accountBodies[tc.accountPath]) != 1 {
				t.Fatalf("master-data lookups = %d, want 1", len(stub.accountBodies[tc.accountPath]))
			}
			if len(stub.susaQueries[tc.susaPath]) != 1 {
				t.Fatalf("SuSa lookups = %d, want 1", len(stub.susaQueries[tc.susaPath]))
			}
		})
	}
}

func TestPersonalAccountBalanceReturnsTypedNotFoundError(t *testing.T) {
	for _, command := range []string{"debitor", "kreditor"} {
		t.Run(command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			withCLI(t, "", false)

			err := run([]string{"--config", configPath, command, "balance", "9999", "--from=2025-01-01", "--to=2025-12-31"})
			want := command + " 9999 not found"
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestPersonalAccountShowReturnsTypedNotFoundError(t *testing.T) {
	for _, command := range []string{"debitor", "kreditor"} {
		t.Run(command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			configPath := sachkontoConfigPath(t, stub.server.URL)
			withCLI(t, "", false)

			err := run([]string{"--config", configPath, command, "show", "9999"})
			want := command + " 9999 not found"
			if err == nil || err.Error() != want {
				t.Fatalf("err = %v, want %q", err, want)
			}
		})
	}
}

func TestPersonalAccountCreatePostsFormAndStitchesResult(t *testing.T) {
	cases := []struct {
		command     string
		path        string
		numberField string
		number      string
		extraArgs   []string
		wantBody    map[string]any
	}{
		{
			command: "kreditor", path: "/rest/createkreditor", numberField: "kreditorNumber", number: "70001",
			extraArgs: []string{"--number=70001", "--number-range=3", "--sum-account=3300"},
			wantBody:  map[string]any{"contactId": 49200.0, "personalAccountNumber": "70001", "numberRangeNumber": 3.0, "sumAccountNumber": "3300"},
		},
		{
			command: "debitor", path: "/rest/createdebitor", numberField: "debitorNumber", number: "10001",
			wantBody: map[string]any{"contactId": 49200.0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			stub.contactByID["49200"] = map[string]any{"id": 49200.0, "lastname": "Neue Firma"}
			stub.createResponses[tc.path] = map[string]any{"number": tc.number}
			accountPath := "/rest/" + tc.command + "accounts"
			stub.accountRecords[accountPath] = []any{
				map[string]any{"number": tc.number, "name": "Neue Firma", "contactId": 49200.0},
			}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)
			args := []string{"--config", configPath, tc.command, "create", "--contact-id=49200"}
			args = append(args, tc.extraArgs...)
			args = append(args, "--yes")

			if err := run(args); err != nil {
				t.Fatal(err)
			}
			bodies := stub.createBodies[tc.path]
			if len(bodies) != 1 {
				t.Fatalf("create requests = %d", len(bodies))
			}
			if !reflect.DeepEqual(bodies[0], tc.wantBody) {
				t.Fatalf("create body = %#v, want %#v", bodies[0], tc.wantBody)
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			if got["status"] != "created" || got["number"] != tc.number || got["contactId"] != 49200.0 {
				t.Fatalf("stdout = %s", output.String())
			}
			if got[tc.command].(map[string]any)["number"] != tc.number {
				t.Fatalf("%s = %#v", tc.command, got[tc.command])
			}
			if got["kontakt"].(map[string]any)[tc.numberField] != tc.number {
				t.Fatalf("kontakt = %#v", got["kontakt"])
			}
		})
	}
}

func TestPersonalAccountCreateReturnsAlreadyExistsWithoutWriting(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.contactByID["49200"] = map[string]any{
		"id": 49200.0, "lastname": "Bestehende Firma", "kreditorNumber": "70002",
	}
	stub.accountRecords["/rest/kreditoraccounts"] = []any{
		map[string]any{"number": "70002", "name": "Bestehende Firma", "contactId": 49200.0},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kreditor", "create", "--contact-id=49200", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if len(stub.createBodies["/rest/createkreditor"]) != 0 {
		t.Fatalf("unexpected create requests = %#v", stub.createBodies)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["status"] != "already_exists" || got["number"] != "70002" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestPersonalAccountCreateDryRunLeavesNumberUnset(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.contactByID["49200"] = map[string]any{"id": 49200.0, "lastname": "Neue Firma"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kreditor", "create", "--contact-id=49200", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if len(stub.createBodies["/rest/createkreditor"]) != 0 {
		t.Fatalf("dry-run wrote %#v", stub.createBodies)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	request := got["request"].(map[string]any)
	if got["status"] != "dry_run" || request["contactId"] != 49200.0 {
		t.Fatalf("stdout = %s", output.String())
	}
	if _, ok := request["personalAccountNumber"]; ok {
		t.Fatalf("personalAccountNumber must be omitted: %#v", request)
	}
}

func TestPersonalAccountCreateDataPreservesFullForm(t *testing.T) {
	stub := newPersonalAccountStub(t)
	stub.contactByID["49200"] = map[string]any{"id": 49200.0, "lastname": "Daten GmbH"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)
	form := `{"contactId":49200,"group":"Inland","currency":"EUR","paymentTermForm":{"name":"14 Tage"}}`

	if err := run([]string{"--config", configPath, "kreditor", "create", "--data", form, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	request := got["request"].(map[string]any)
	if request["group"] != "Inland" || request["currency"] != "EUR" {
		t.Fatalf("request = %#v", request)
	}
	term := request["paymentTermForm"].(map[string]any)
	if term["name"] != "14 Tage" {
		t.Fatalf("paymentTermForm = %#v", term)
	}
}

func TestPersonalAccountUpdatePostsFileAndReadsBackUpdatedAccount(t *testing.T) {
	cases := []struct {
		command string
		number  string
		path    string
		changes map[string]any
	}{
		{
			command: "debitor",
			number:  "10000",
			path:    "/rest/debitoraccounts/10000",
			changes: map[string]any{"vatCode": "EUmID", "sumAccountNumber": "1400"},
		},
		{
			command: "kreditor",
			number:  "70000",
			path:    "/rest/kreditoraccounts/70000",
			changes: map[string]any{"paymentTermId": 42.0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			stub := newPersonalAccountStub(t)
			accountPath := "/rest/" + tc.command + "accounts"
			account := map[string]any{"number": tc.number, "name": "Updated"}
			for key, value := range tc.changes {
				account[key] = value
			}
			stub.accountRecords[accountPath] = []any{account}
			configPath := sachkontoConfigPath(t, stub.server.URL)
			raw, err := json.Marshal(tc.changes)
			if err != nil {
				t.Fatal(err)
			}
			filePath := filepath.Join(t.TempDir(), "changes.json")
			if err := os.WriteFile(filePath, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			output, _ := withCLI(t, "", false)

			if err := run([]string{"--config", configPath, tc.command, "update", tc.number, "--file=" + filePath, "--yes"}); err != nil {
				t.Fatal(err)
			}

			bodies := stub.updateBodies[tc.path]
			if len(bodies) != 1 {
				t.Fatalf("update requests = %d", len(bodies))
			}
			if !reflect.DeepEqual(bodies[0], tc.changes) {
				t.Fatalf("update body = %#v, want %#v", bodies[0], tc.changes)
			}
			readbacks := stub.accountBodies[accountPath]
			if len(readbacks) != 1 {
				t.Fatalf("readback requests = %d", len(readbacks))
			}
			fields, ok := readbacks[0]["fields"].([]any)
			if !ok {
				t.Fatalf("readback fields = %#v", readbacks[0]["fields"])
			}
			for _, want := range append([]string{"number"}, mapKeys(tc.changes)...) {
				found := false
				for _, field := range fields {
					if field == want {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("readback fields %v missing %q", fields, want)
				}
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			if got["status"] != "updated" || got["number"] != tc.number {
				t.Fatalf("stdout = %s", output.String())
			}
			if !reflect.DeepEqual(got[tc.command], account) {
				t.Fatalf("%s = %#v, want %#v", tc.command, got[tc.command], account)
			}
		})
	}
}

func TestPersonalAccountUpdateDryRunDoesNotWrite(t *testing.T) {
	stub := newPersonalAccountStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	filePath := filepath.Join(t.TempDir(), "changes.json")
	if err := os.WriteFile(filePath, []byte(`{"vatCode":"INL"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "debitor", "update", "10000", "--file=" + filePath, "--dry-run"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.updateBodies) != 0 {
		t.Fatalf("dry-run wrote %#v", stub.updateBodies)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "dry_run" || got["endpoint"] != "POST /debitoraccounts/10000" {
		t.Fatalf("stdout = %s", output.String())
	}
	if !reflect.DeepEqual(got["request"], map[string]any{"vatCode": "INL"}) {
		t.Fatalf("request = %#v", got["request"])
	}
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
