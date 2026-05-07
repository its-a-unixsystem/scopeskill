package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type offenePostenStub struct {
	server          *httptest.Server
	openItemBodies  map[string][]map[string]any
	openItemRecords map[string][]any
	accountBodies   map[string][]map[string]any
	accountRecords  map[string][]any
	contactByID     map[string]map[string]any
	belegByPath     map[string]map[string]any
}

func newOffenePostenStub(t *testing.T) *offenePostenStub {
	t.Helper()
	stub := &offenePostenStub{
		openItemBodies:  map[string][]map[string]any{},
		openItemRecords: map[string][]any{},
		accountBodies:   map[string][]map[string]any{},
		accountRecords:  map[string][]any{},
		contactByID:     map[string]map[string]any{},
		belegByPath:     map[string]map[string]any{},
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case (r.URL.Path == "/rest/openitems/debtors" || r.URL.Path == "/rest/openitems/creditors") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.openItemBodies[r.URL.Path] = append(stub.openItemBodies[r.URL.Path], body)
			records := stub.openItemRecords[r.URL.Path]
			if records == nil {
				records = []any{}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
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
		case strings.HasPrefix(r.URL.Path, "/rest/contact/"):
			id := strings.TrimPrefix(r.URL.Path, "/rest/contact/")
			rec, ok := stub.contactByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusBadRequest)
				return
			}
			writeJSONForCLI(w, rec)
		case strings.HasPrefix(r.URL.Path, "/rest/incominginvoice/") ||
			strings.HasPrefix(r.URL.Path, "/rest/outgoinginvoice/") ||
			strings.HasPrefix(r.URL.Path, "/rest/credit/"):
			rec, ok := stub.belegByPath[r.URL.Path]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			writeJSONForCLI(w, rec)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestOffenePostenListRequiresSeite(t *testing.T) {
	stub := newOffenePostenStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "offene-posten", "list"})
	if err == nil || !strings.Contains(err.Error(), "missing required --seite") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.openItemBodies) != 0 {
		t.Fatalf("requests = %#v", stub.openItemBodies)
	}
}

func TestOffenePostenListDebitorWithKontoFilter(t *testing.T) {
	stub := newOffenePostenStub(t)
	stub.openItemRecords["/rest/openitems/debtors"] = []any{
		map[string]any{"leadRowNumber": 11.0, "accountNumber": "10000", "invoiceNumber": "2026-3"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "list", "--seite=debitor", "--konto=10000"}); err != nil {
		t.Fatal(err)
	}

	bodies := stub.openItemBodies["/rest/openitems/debtors"]
	if len(bodies) != 1 {
		t.Fatalf("debtor open-item requests = %d", len(bodies))
	}
	cond := bodies[0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "accountNumber" || cond["operator"] != "equals" || cond["value"] != "10000" {
		t.Fatalf("condition = %#v", cond)
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got[0].(map[string]any)["accountNumber"] != "10000" {
		t.Fatalf("records = %#v", got)
	}
}

func TestOffenePostenListOverdueUsesTodayMillis(t *testing.T) {
	stub := newOffenePostenStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	fixedNow(t, time.Date(2026, 5, 7, 15, 30, 0, 0, time.FixedZone("CEST", 2*60*60)))
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "list", "--seite=debitor", "--overdue"}); err != nil {
		t.Fatal(err)
	}

	cond := stub.openItemBodies["/rest/openitems/debtors"][0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "dueDate" || cond["operator"] != "less" || cond["value"] != float64(1778112000000) {
		t.Fatalf("condition = %#v", cond)
	}
}

func TestOffenePostenListKreditorDueBefore(t *testing.T) {
	stub := newOffenePostenStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "list", "--seite=kreditor", "--due-before=2026-06-30"}); err != nil {
		t.Fatal(err)
	}

	cond := stub.openItemBodies["/rest/openitems/creditors"][0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "dueDate" || cond["operator"] != "less" || cond["value"] != float64(1782777600000) {
		t.Fatalf("condition = %#v", cond)
	}
}

func TestOffenePostenListKontaktIDResolvesToSideAccountNumber(t *testing.T) {
	stub := newOffenePostenStub(t)
	stub.contactByID["49198"] = map[string]any{"id": 49198.0, "lastname": "Looka", "kreditorNumber": "70010"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "list", "--seite=kreditor", "--kontakt-id=49198"}); err != nil {
		t.Fatal(err)
	}

	cond := stub.openItemBodies["/rest/openitems/creditors"][0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "accountNumber" || cond["operator"] != "equals" || cond["value"] != "70010" {
		t.Fatalf("condition = %#v", cond)
	}
}

func TestOffenePostenShowStitchesOPBelegAndKontakt(t *testing.T) {
	stub := newOffenePostenStub(t)
	stub.openItemRecords["/rest/openitems/creditors"] = []any{
		map[string]any{"leadRowNumber": 31995.0, "accountNumber": "70010", "invoiceNumber": "2026-1"},
	}
	stub.accountRecords["/rest/kreditoraccounts"] = []any{
		map[string]any{"number": "70010", "name": "Looka", "contactId": 49198.0},
	}
	stub.contactByID["49198"] = map[string]any{"id": 49198.0, "lastname": "Looka", "kreditorNumber": "70010"}
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{"number": "2026-1", "documentNumber": "2HUJ9MS4-0002"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "offene-posten", "show", "31995"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["op"].(map[string]any)["leadRowNumber"] != float64(31995) {
		t.Fatalf("op = %#v", got["op"])
	}
	if got["beleg"].(map[string]any)["documentNumber"] != "2HUJ9MS4-0002" {
		t.Fatalf("beleg = %#v", got["beleg"])
	}
	if got["kontakt"].(map[string]any)["lastname"] != "Looka" {
		t.Fatalf("kontakt = %#v", got["kontakt"])
	}
	if len(stub.openItemBodies["/rest/openitems/debtors"]) != 1 || len(stub.openItemBodies["/rest/openitems/creditors"]) != 1 {
		t.Fatalf("open-item lookups = %#v", stub.openItemBodies)
	}
}
