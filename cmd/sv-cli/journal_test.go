package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type journalStub struct {
	server         *httptest.Server
	journalBodies  []map[string]any
	journalRecords []any
	journalByID    map[string][]any
	belegByPath    map[string]map[string]any
	journalSearch  func(map[string]any) []any
}

func newJournalStub(t *testing.T) *journalStub {
	t.Helper()
	stub := &journalStub{
		journalRecords: []any{},
		journalByID:    map[string][]any{},
		belegByPath:    map[string]map[string]any{},
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case r.URL.Path == "/rest/journal" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.journalBodies = append(stub.journalBodies, body)
			records := stub.journalRecords
			if stub.journalSearch != nil {
				records = stub.journalSearch(body)
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(r.URL.Path, "/rest/journal/")
			records, ok := stub.journalByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": records})
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
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func TestJournalSearchComposesAllFilters(t *testing.T) {
	stub := newJournalStub(t)
	stub.journalRecords = []any{
		map[string]any{"documentNumber": "2025-000001", "accountNumber": "4400"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{
		"--config", configPath,
		"journal", "search",
		"--from=2025-01-01",
		"--to=2025-01-31",
		"--konto=4400",
		"--text=Rabatt",
		"--belegnr=2025-000001",
		"--amount-min=10.5",
		"--amount-max=20",
		"--dim=kostenstelle=K100",
		"--dim=projekt=P42",
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(stub.journalBodies) != 1 {
		t.Fatalf("journal requests = %d", len(stub.journalBodies))
	}
	body := stub.journalBodies[0]
	if body["postingDateSince"] != float64(1735689600000) {
		t.Fatalf("postingDateSince = %#v", body["postingDateSince"])
	}
	if body["postingDateBefore"] != float64(1738368000000) {
		t.Fatalf("postingDateBefore = %#v", body["postingDateBefore"])
	}
	search := body["search"].([]any)
	assertCondition(t, search, "accountNumber", "equals", "4400")
	assertCondition(t, search, "postingText", "contains", "Rabatt")
	assertCondition(t, search, "documentNumber", "equals", "2025-000001")
	assertCondition(t, search, "amount", "greaterorequals", float64(10.5))
	assertCondition(t, search, "amount", "lessorequals", float64(20))
	assertCondition(t, search, "documentDimension_1", "equals", "K100")
	assertCondition(t, search, "documentDimension_3", "equals", "P42")

	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if got[0].(map[string]any)["documentNumber"] != "2025-000001" {
		t.Fatalf("records = %#v", got)
	}
}

func TestJournalSearchDataWithAllIsRejected(t *testing.T) {
	stub := newJournalStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "journal", "search", "--data", `{"page":0}`, "--all"})
	if err == nil || !strings.Contains(err.Error(), "--data cannot be combined") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.journalBodies) != 0 {
		t.Fatalf("requests = %#v", stub.journalBodies)
	}
}

func TestJournalSearchRejectsBadDimension(t *testing.T) {
	stub := newJournalStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "journal", "search", "--dim=unknown=K100"})
	if err == nil || !strings.Contains(err.Error(), "unknown --dim key") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuchungShowStitchesLinesDimensionsAndBeleg(t *testing.T) {
	stub := newJournalStub(t)
	stub.journalByID["2025-000001"] = []any{
		map[string]any{
			"documentNumber":         "2025-000001",
			"postingDate":            "2025-06-02",
			"documentText":           "Fleetcor Gebühr",
			"internalDocumentNumber": "2025-4",
			"accountNumber":          "6530",
			"debitAmount":            1.95,
			"creditAmount":           0.0,
			"vatKey":                 "V19",
			"documentDimension_1":    111.0,
			"pdeRowNumber":           "9001",
		},
		map[string]any{
			"documentNumber":         "2025-000001",
			"internalDocumentNumber": "2025-4",
			"accountNumber":          "3300",
			"debitAmount":            0.0,
			"creditAmount":           2.32,
			"pdeRowNumber":           "9001",
		},
	}
	stub.belegByPath["/rest/incominginvoice/2025-4"] = map[string]any{"number": "2025-4", "postingDocumentNumber": "2025-000001"}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "show", "2025-000001"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	buchung := got["buchung"].(map[string]any)
	if buchung["documentNumber"] != "2025-000001" {
		t.Fatalf("buchung = %#v", buchung)
	}
	if len(buchung["lines"].([]any)) != 2 {
		t.Fatalf("lines = %#v", buchung["lines"])
	}
	dimensions := buchung["dimensions"].(map[string]any)
	if dimensions["dimension_1"] != float64(111) {
		t.Fatalf("dimensions = %#v", dimensions)
	}
	if got["beleg"].(map[string]any)["postingDocumentNumber"] != "2025-000001" {
		t.Fatalf("beleg = %#v", got["beleg"])
	}
	lifecycle := got["lifecycle"].(map[string]any)
	if lifecycle["state"] != "active" {
		t.Fatalf("lifecycle = %#v", lifecycle)
	}
}

func TestBuchungShowReportsCancelledStateAndReplacement(t *testing.T) {
	stub := newJournalStub(t)
	stub.journalByID["P-2025-1"] = cancelOriginalRows()
	stub.journalByID["S-2025-1"] = cancelStornoRows()
	replacement := cancelOriginalRows()
	for _, item := range replacement {
		record := item.(map[string]any)
		record["documentNumber"] = "R-2025-1"
		record["cancellationNumber"] = "9001"
	}
	stub.journalByID["R-2025-1"] = replacement
	stub.journalRecords = []any{
		map[string]any{"documentNumber": "S-2025-1", "cancellationNumber": "9001"},
		map[string]any{"documentNumber": "R-2025-1", "cancellationNumber": "9001"},
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "show", "P-2025-1"}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	lifecycle := got["lifecycle"].(map[string]any)
	if lifecycle["state"] != "cancelled" || lifecycle["cancellationDocumentNumber"] != "S-2025-1" || lifecycle["replacementDocumentNumber"] != "R-2025-1" {
		t.Fatalf("lifecycle = %#v", lifecycle)
	}
	cancellation := lifecycle["cancellation"].(map[string]any)
	if cancellation["documentNumber"] != "S-2025-1" || len(cancellation["rows"].([]any)) != 2 {
		t.Fatalf("cancellation = %#v", cancellation)
	}
	if got["buchung"] == nil {
		t.Fatal("buchung output is missing")
	}
}

func TestBuchungShowRejectsUnverifiedCancellationLinkage(t *testing.T) {
	stub := newJournalStub(t)
	stub.journalByID["P-2025-1"] = cancelOriginalRows()
	linked := cancelOriginalRows()
	for _, item := range linked {
		record := item.(map[string]any)
		record["documentNumber"] = "X-2025-1"
		record["cancellationNumber"] = "9001"
	}
	stub.journalByID["X-2025-1"] = linked
	stub.journalRecords = []any{map[string]any{"documentNumber": "X-2025-1", "cancellationNumber": "9001"}}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "show", "P-2025-1"})
	if err == nil || !strings.Contains(err.Error(), "none is a verified sign-reversed Storno") {
		t.Fatalf("error = %v", err)
	}

	var got map[string]any
	if jsonErr := json.Unmarshal(output.Bytes(), &got); jsonErr != nil {
		t.Fatalf("stdout JSON: %v: %s", jsonErr, output.String())
	}
	lifecycle := got["lifecycle"].(map[string]any)
	if lifecycle["state"] != "conflict" || got["buchung"] == nil {
		t.Fatalf("conflict output = %#v", got)
	}
}

func TestBuchungShowMapsMissingToNotFoundMessage(t *testing.T) {
	stub := newJournalStub(t)
	configPath := sachkontoConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "show", "missing"})
	if err == nil || !strings.Contains(err.Error(), notFoundOrUnauthorisedMessage) {
		t.Fatalf("error = %v", err)
	}
}

func assertCondition(t *testing.T, search []any, field, operator string, value any) {
	t.Helper()
	for _, item := range search {
		cond := item.(map[string]any)
		if cond["field"] == field && cond["operator"] == operator && cond["value"] == value {
			return
		}
	}
	t.Fatalf("missing condition field=%s operator=%s value=%#v in %#v", field, operator, value, search)
}

func TestJournalSearchAllPreservesDocumentAcrossPageBoundary(t *testing.T) {
	stub := newJournalStub(t)
	rows := make([]any, 1003)
	for i := range rows {
		document := "preceding"
		if i >= 999 {
			document = "boundary-document"
		}
		rows[i] = map[string]any{"rowNumber": i + 1, "documentNumber": document, "postingDate": 1748822400000}
	}
	stub.journalSearch = func(body map[string]any) []any {
		if after, ok := body["afterId"].(float64); ok {
			if body["order"] != nil || body["page"] != float64(0) {
				t.Errorf("invalid cursor request: %#v", body)
			}
			page := []any{}
			for _, row := range rows {
				if row.(map[string]any)["rowNumber"].(int) > int(after) && len(page) < 1000 {
					page = append(page, row)
				}
			}
			return page
		}
		// Offset paging moves the tied document's first line behind its other
		// lines after page zero, repeating it and losing a distinct line.
		page := int(body["page"].(float64))
		if page == 0 {
			return rows[:1000]
		}
		if page == 1 {
			return []any{rows[1001], rows[1002], rows[999]}
		}
		return []any{}
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)
	if err := run([]string{"--config", configPath, "journal", "search", "--all"}); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1003 {
		t.Fatalf("rows = %d, want 1003", len(got))
	}
	documentLines := map[float64]int{}
	for i, row := range got {
		if row["rowNumber"] != float64(i+1) {
			t.Fatalf("row %d = %#v", i+1, row)
		}
		if row["documentNumber"] == "boundary-document" {
			documentLines[row["rowNumber"].(float64)]++
		}
	}
	for id := float64(1000); id <= 1003; id++ {
		if documentLines[id] != 1 {
			t.Fatalf("document line %.0f appears %d times", id, documentLines[id])
		}
	}
}

func TestJournalSearchAllIntegrityErrorHasNoOutput(t *testing.T) {
	stub := newJournalStub(t)
	stub.journalSearch = func(body map[string]any) []any {
		if body["afterId"] == float64(0) {
			rows := make([]any, 1000)
			for i := range rows {
				rows[i] = map[string]any{"rowNumber": i + 1}
			}
			return rows
		}
		return []any{map[string]any{"rowNumber": 1000}}
	}
	configPath := sachkontoConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)
	err := run([]string{"--config", configPath, "journal", "search", "--all"})
	if err == nil || !strings.Contains(err.Error(), "unsafe pagination: rowNumber must increase") {
		t.Fatalf("err = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("partial stdout: %s", output.String())
	}
}
