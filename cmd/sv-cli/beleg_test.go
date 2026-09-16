package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type belegStub struct {
	server          *httptest.Server
	searchBodies    map[string][]map[string]any
	searchRecords   map[string][]any
	kreditorBodies  []map[string]any
	kreditorRecords []any
	contactByID     map[string]map[string]any
	belegByPath     map[string]map[string]any
	belegGets       int
	updateBodies    []map[string]any
	updatePaths     []string
	updateStatus    int
	applyUpdate     bool
	ambiguousUpdate bool
}

func newBelegStub(t *testing.T) *belegStub {
	t.Helper()
	stub := &belegStub{
		searchBodies:  map[string][]map[string]any{},
		searchRecords: map[string][]any{},
		contactByID:   map[string]map[string]any{},
		belegByPath:   map[string]map[string]any{},
		updateStatus:  http.StatusOK,
		applyUpdate:   true,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case (r.URL.Path == "/rest/incominginvoices" || r.URL.Path == "/rest/credits") && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.searchBodies[r.URL.Path] = append(stub.searchBodies[r.URL.Path], body)
			records := stub.searchRecords[r.URL.Path]
			if records == nil {
				records = []any{}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case r.URL.Path == "/rest/kreditoraccounts" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.kreditorBodies = append(stub.kreditorBodies, body)
			if stub.kreditorRecords == nil {
				writeJSONForCLI(w, map[string]any{"records": []any{}})
				return
			}
			writeJSONForCLI(w, map[string]any{"records": stub.kreditorRecords})
		case strings.HasPrefix(r.URL.Path, "/rest/contact/"):
			id := strings.TrimPrefix(r.URL.Path, "/rest/contact/")
			rec, ok := stub.contactByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusBadRequest)
				return
			}
			writeJSONForCLI(w, rec)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/rest/incominginvoice/"):
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.updateBodies = append(stub.updateBodies, body)
			stub.updatePaths = append(stub.updatePaths, r.URL.Path)
			if stub.applyUpdate {
				if rec := stub.belegByPath[r.URL.Path]; rec != nil {
					for field, value := range body {
						rec[field] = value
					}
				}
			}
			if stub.ambiguousUpdate {
				hijacker := w.(http.Hijacker)
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
				return
			}
			if stub.updateStatus != http.StatusOK {
				http.Error(w, "provider rejected update", stub.updateStatus)
				return
			}
			writeJSONForCLI(w, map[string]any{"success": true})
		case strings.HasPrefix(r.URL.Path, "/rest/incominginvoice/") ||
			strings.HasPrefix(r.URL.Path, "/rest/credit/"):
			stub.belegGets++
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

func TestEingangsrechnungUpdateSendsOnlyRequestedFieldsAsEpochMillis(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":          "2026-1",
		"documentNumber":  "INV-OLD",
		"vendorContactId": nil,
		"documentDate":    1767225600000.0,
		"dueDate":         1771027200000.0,
		"text1":           "old text",
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--vendor-contact-id=49198",
		"--document-number=INV-NEW",
		"--document-date=2026-04-01",
		"--due-date=2026-05-01",
		"--delivery-date-from=2026-04-01",
		"--delivery-date-to=2026-04-30",
		"--text=Repariert nach Posteingang",
		"--yes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stub.updateBodies) != 1 || len(stub.updatePaths) != 1 {
		t.Fatalf("update requests = %d (%v)", len(stub.updateBodies), stub.updatePaths)
	}
	if stub.updatePaths[0] != "/rest/incominginvoice/2026-1" {
		t.Fatalf("update path = %q", stub.updatePaths[0])
	}
	body := stub.updateBodies[0]
	if len(body) != 7 {
		t.Fatalf("update body should carry only the 7 requested fields: %#v", body)
	}
	want := map[string]any{
		"vendorContactId":  49198.0,
		"documentNumber":   "INV-NEW",
		"documentDate":     1775001600000.0,
		"dueDate":          1777593600000.0,
		"deliveryDateFrom": 1775001600000.0,
		"deliveryDateTo":   1777507200000.0,
		"text1":            "Repariert nach Posteingang",
	}
	for field, expected := range want {
		if body[field] != expected {
			t.Fatalf("body[%q] = %#v, want %#v", field, body[field], expected)
		}
	}
}

func TestEingangsrechnungUpdateAppliesVendorContactIDChange(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":          "2026-1",
		"vendorContactId": 49199.0,
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--vendor-contact-id=49198",
		"--yes",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(stub.updateBodies) != 1 {
		t.Fatalf("update requests = %d", len(stub.updateBodies))
	}
	if stub.updateBodies[0]["vendorContactId"] != 49198.0 {
		t.Fatalf("body = %#v", stub.updateBodies[0])
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "updated" {
		t.Fatalf("status = %v", status["status"])
	}
	if got := stub.belegByPath["/rest/incominginvoice/2026-1"]["vendorContactId"]; got != 49198.0 {
		t.Fatalf("vendorContactId after update = %#v", got)
	}
}

func TestEingangsrechnungUpdateAlreadyUpToDateSendsNoWrite(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":          "2026-1",
		"vendorContactId": 49198.0,
		"documentNumber":  "INV-1",
		"documentDate":    1775001600000.0,
		"text1":           "same text",
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--vendor-contact-id=49198",
		"--document-number=INV-1",
		"--document-date=2026-04-01",
		"--text=same text",
		"--yes",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(stub.updateBodies) != 0 {
		t.Fatalf("update requests = %d; already up to date must not write", len(stub.updateBodies))
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "already_up_to_date" {
		t.Fatalf("status = %v", status["status"])
	}
}

func TestEingangsrechnungUpdateDryRunWritesNothing(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":       "2026-1",
		"documentDate": 0.0,
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, stderr := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--document-date=2026-04-01",
		"--dry-run",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(stub.updateBodies) != 0 {
		t.Fatalf("dry-run must not write; got %d requests", len(stub.updateBodies))
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "dry_run" {
		t.Fatalf("status = %v", status["status"])
	}
	if !strings.Contains(stderr.String(), "/incominginvoice/2026-1") {
		t.Fatalf("preview missing endpoint: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "1775001600000") {
		t.Fatalf("preview missing epoch payload: %q", stderr.String())
	}
}

func TestEingangsrechnungUpdateRequiresTTYOrYes(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":       "2026-1",
		"documentDate": 0.0,
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--document-date=2026-04-01",
	})
	if err == nil || !strings.Contains(err.Error(), "TTY or --yes") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.updateBodies) != 0 {
		t.Fatalf("no write may happen before confirmation; got %d requests", len(stub.updateBodies))
	}
}

func TestEingangsrechnungUpdateRoundTripsProviderBerlinDate(t *testing.T) {
	stub := newBelegStub(t)
	// Provider anchored the same day at Europe/Berlin midnight:
	// 2026-04-01T00:00:00+02:00 = 1774994400000, while the CLI sends
	// UTC-midnight 1775001600000. Both are calendar day 2026-04-01.
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":       "2026-1",
		"documentDate": 1774994400000.0,
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--document-date=2026-04-01",
		"--text=any change",
		"--yes",
	})
	if err != nil {
		t.Fatal(err)
	}

	if stub.belegByPath["/rest/incominginvoice/2026-1"]["documentDate"] != 1775001600000.0 {
		t.Fatalf("documentDate after update = %#v", stub.belegByPath["/rest/incominginvoice/2026-1"]["documentDate"])
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "updated" {
		t.Fatalf("status = %v (%#v)", status["status"], status)
	}
}

func TestEingangsrechnungUpdateReportsVerificationRequiredWhenFieldNotApplied(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/2026-1"] = map[string]any{
		"number":       "2026-1",
		"documentDate": 0.0,
	}
	stub.applyUpdate = false // provider accepts the write but does not apply it
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "update", "2026-1",
		"--document-date=2026-04-01",
		"--yes",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	status := stdoutStatus(t, output.String())
	if status["status"] != "verification_required" {
		t.Fatalf("status = %v", status["status"])
	}
	if !strings.Contains(err.Error(), "documentDate") {
		t.Fatalf("error should name the unapplied field: %v", err)
	}
}

func TestEingangsrechnungUpdateRejectsInvalidDateAndMissingFlags(t *testing.T) {
	stub := newBelegStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "eingangsrechnung", "update", "2026-1", "--document-date=04/01/2026", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Fatalf("error = %v", err)
	}
	err = run([]string{"--config", configPath, "eingangsrechnung", "update", "2026-1", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "at least one field flag") {
		t.Fatalf("error = %v", err)
	}
	err = run([]string{"--config", configPath, "eingangsrechnung", "update", "--vendor-contact-id=1", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "exactly one idOrNumber") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.updateBodies) != 0 || stub.belegGets != 0 {
		t.Fatalf("requests made: updates=%d gets=%d", len(stub.updateBodies), stub.belegGets)
	}
}

func TestGutschriftRejectsUpdateSubcommand(t *testing.T) {
	stub := newBelegStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "gutschrift", "update", "GS-1", "--text=x", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "unknown gutschrift command: update") {
		t.Fatalf("error = %v", err)
	}
}

func TestEingangsrechnungSearchPostsExpectedConditions(t *testing.T) {
	stub := newBelegStub(t)
	stub.searchRecords["/rest/incominginvoices"] = []any{
		map[string]any{"documentNumber": "ER-2026-1", "vendorName": "Looka"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"eingangsrechnung", "search",
		"--document-number=ER-2026-1",
		"--vendor-name=Looka",
		"--content-state=10",
		"--payment-state=20",
		"--posting-state=30",
	})
	if err != nil {
		t.Fatal(err)
	}

	bodies := stub.searchBodies["/rest/incominginvoices"]
	if len(bodies) != 1 {
		t.Fatalf("incoming invoice requests = %d", len(bodies))
	}
	search := bodies[0]["search"].([]any)
	want := []struct {
		field    string
		operator string
		value    string
	}{
		{"documentNumber", "equals", "ER-2026-1"},
		{"vendorName", "contains", "Looka"},
		{"contentStateId", "equals", "10"},
		{"paymentStateId", "equals", "20"},
		{"postingStateId", "equals", "30"},
	}
	if len(search) != len(want) {
		t.Fatalf("conditions = %#v", search)
	}
	for i, w := range want {
		cond := search[i].(map[string]any)
		if cond["field"] != w.field || cond["operator"] != w.operator || cond["value"] != w.value {
			t.Fatalf("condition %d = %#v", i, cond)
		}
	}
	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got[0].(map[string]any)["documentNumber"] != "ER-2026-1" {
		t.Fatalf("records = %#v", got)
	}
}

func TestGutschriftSearchDataOverridePostsRawBody(t *testing.T) {
	stub := newBelegStub(t)
	stub.searchRecords["/rest/credits"] = []any{
		map[string]any{"documentNumber": "GS-2026-1"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	dataPath := filepath.Join(t.TempDir(), "body.json")
	if err := os.WriteFile(dataPath, []byte(`{"page":2,"pageSize":7,"fields":["documentNumber"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "gutschrift", "search", "--data", "@" + dataPath}); err != nil {
		t.Fatal(err)
	}

	bodies := stub.searchBodies["/rest/credits"]
	if len(bodies) != 1 {
		t.Fatalf("credit requests = %d", len(bodies))
	}
	body := bodies[0]
	if body["page"].(float64) != 2 || body["pageSize"].(float64) != 7 {
		t.Fatalf("body = %#v", body)
	}
	if !strings.Contains(output.String(), `"documentNumber": "GS-2026-1"`) {
		t.Fatalf("stdout = %q", output.String())
	}
}

func TestGutschriftSearchAllPaginatesCredits(t *testing.T) {
	stub := newBelegStub(t)
	full := make([]any, scopeskill.MaxSearchPageSize)
	for i := range full {
		full[i] = map[string]any{"documentNumber": "GS"}
	}
	stub.searchRecords["/rest/credits"] = full
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "gutschrift", "search", "--vendor-name=Looka", "--all", "--max=1001"}); err != nil {
		t.Fatal(err)
	}

	bodies := stub.searchBodies["/rest/credits"]
	if len(bodies) != 2 {
		t.Fatalf("credit requests = %d", len(bodies))
	}
	for i, body := range bodies {
		if body["page"].(float64) != float64(i) || body["pageSize"].(float64) != float64(scopeskill.MaxSearchPageSize) {
			t.Fatalf("call %d body = %#v", i, body)
		}
	}
}

func TestEingangsrechnungShowStitchesKontaktFromVendorContactID(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/incominginvoice/ER-2026-1"] = map[string]any{
		"documentNumber":          "ER-2026-1",
		"vendorContactId":         49198.0,
		"vendorPersonalAccountId": 70010.0,
		"vendorPersonalAccountNo": "70010",
	}
	stub.contactByID["49198"] = map[string]any{"id": 49198.0, "lastname": "Looka", "kreditorNumber": "70010"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "eingangsrechnung", "show", "ER-2026-1"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.kreditorBodies) != 0 {
		t.Fatalf("kreditor fallback should not run: %#v", stub.kreditorBodies)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["eingangsrechnung"].(map[string]any)["documentNumber"] != "ER-2026-1" {
		t.Fatalf("eingangsrechnung = %#v", got["eingangsrechnung"])
	}
	if got["kontakt"].(map[string]any)["lastname"] != "Looka" {
		t.Fatalf("kontakt = %#v", got["kontakt"])
	}
}

func TestGutschriftShowFallsBackThroughKreditorAccount(t *testing.T) {
	stub := newBelegStub(t)
	stub.belegByPath["/rest/credit/GS-2026-1"] = map[string]any{
		"documentNumber":          "GS-2026-1",
		"vendorContactId":         nil,
		"vendorPersonalAccountId": 70010.0,
	}
	stub.kreditorRecords = []any{
		map[string]any{"id": 70010.0, "number": "70010", "contactId": 49198.0},
	}
	stub.contactByID["49198"] = map[string]any{"id": 49198.0, "lastname": "Looka", "kreditorNumber": "70010"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "gutschrift", "show", "GS-2026-1"}); err != nil {
		t.Fatal(err)
	}

	if len(stub.kreditorBodies) != 1 {
		t.Fatalf("kreditor fallback calls = %d", len(stub.kreditorBodies))
	}
	cond := stub.kreditorBodies[0]["search"].([]any)[0].(map[string]any)
	if cond["field"] != "id" || cond["operator"] != "equals" || cond["value"] != float64(70010) {
		t.Fatalf("fallback condition = %#v", cond)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got["gutschrift"].(map[string]any)["documentNumber"] != "GS-2026-1" {
		t.Fatalf("gutschrift = %#v", got["gutschrift"])
	}
	if got["kontakt"].(map[string]any)["lastname"] != "Looka" {
		t.Fatalf("kontakt = %#v", got["kontakt"])
	}
}

func TestBelegSearchRejectsDataWithPagination(t *testing.T) {
	stub := newBelegStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "eingangsrechnung", "search", "--data", `{"page":0}`, "--all"})
	if err == nil || !strings.Contains(err.Error(), "--data cannot be combined") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.searchBodies) != 0 {
		t.Fatalf("requests = %#v", stub.searchBodies)
	}
}
