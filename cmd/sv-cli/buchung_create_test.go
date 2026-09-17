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

type postingStub struct {
	server                *httptest.Server
	hits                  []string
	journalQueries        []string
	postingBodies         []map[string]any
	fiscalYears           any
	sachkonten            map[string]map[string]any
	debitoren             map[string]map[string]any
	kreditoren            map[string]map[string]any
	vatKeys               []string
	journalByID           map[string][]any
	journalSearchRows     []any
	journalSearchBodies   []map[string]any
	journalSearchPages    map[int][]any
	postingResponse       any
	postingStatus         int
	postingDropConnection bool
	afterWrite            func()
}

func newPostingStub(t *testing.T) *postingStub {
	t.Helper()
	stub := &postingStub{
		sachkonten:        map[string]map[string]any{},
		debitoren:         map[string]map[string]any{},
		kreditoren:        map[string]map[string]any{},
		journalByID:       map[string][]any{},
		journalSearchRows: []any{},
	}
	searchNumber := func(r *http.Request) string {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		for _, item := range body["search"].([]any) {
			cond := item.(map[string]any)
			if cond["field"] == "number" {
				return cond["value"].(string)
			}
		}
		return ""
	}
	recordsFor := func(table map[string]map[string]any, number string) []any {
		if rec, ok := table[number]; ok {
			return []any{rec}
		}
		return []any{}
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.hits = append(stub.hits, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case r.URL.Path == "/rest/fiscalyears" && r.Method == http.MethodGet:
			writeJSONForCLI(w, map[string]any{"years": stub.fiscalYears})
		case r.URL.Path == "/rest/impersonalaccounts" && r.Method == http.MethodPost:
			writeJSONForCLI(w, map[string]any{"records": recordsFor(stub.sachkonten, searchNumber(r))})
		case r.URL.Path == "/rest/debitoraccounts" && r.Method == http.MethodPost:
			writeJSONForCLI(w, map[string]any{"records": recordsFor(stub.debitoren, searchNumber(r))})
		case r.URL.Path == "/rest/kreditoraccounts" && r.Method == http.MethodPost:
			writeJSONForCLI(w, map[string]any{"records": recordsFor(stub.kreditoren, searchNumber(r))})
		case r.URL.Path == "/rest/vatmatrixentries" && r.Method == http.MethodGet:
			var records []any
			for _, key := range stub.vatKeys {
				records = append(records, map[string]any{"vatKey": key})
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case r.URL.Path == "/rest/journal" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.journalSearchBodies = append(stub.journalSearchBodies, body)
			records := stub.journalSearchRows
			if stub.journalSearchPages != nil {
				page, _ := body["page"].(float64)
				records = stub.journalSearchPages[int(page)]
				if records == nil {
					records = []any{}
				}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && r.Method == http.MethodGet:
			stub.journalQueries = append(stub.journalQueries, r.URL.RawQuery)
			id := strings.TrimPrefix(r.URL.Path, "/rest/journal/")
			records, ok := stub.journalByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case r.URL.Path == "/rest/postings/new" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.postingBodies = append(stub.postingBodies, body)
			if stub.afterWrite != nil {
				stub.afterWrite()
			}
			if stub.postingDropConnection {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return
				}
				_ = conn.Close()
				return
			}
			status := stub.postingStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if stub.postingResponse != nil {
				_ = json.NewEncoder(w).Encode(stub.postingResponse)
			}
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *postingStub) writeCount() int {
	return len(s.postingBodies)
}

func (s *postingStub) journalGetCount() int {
	count := 0
	for _, hit := range s.hits {
		if strings.HasPrefix(hit, "GET /rest/journal/") {
			count++
		}
	}
	return count
}

func postingConfigPath(t *testing.T, baseURL string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + baseURL + "\nCUSTOMER=1234567\nSKR=skr04\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return configPath
}

func postingFixture(t *testing.T, input any) string {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "buchung.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

var validPostingInput = map[string]any{
	"documentNumber": "P-2025-1",
	"postingDate":    "2025-06-02",
	"rows": []any{
		map[string]any{"account": "4400", "amount": 119.00, "vatKey": "U19"},
		map[string]any{"account": "1200", "amount": -119.00},
	},
}

func newHappyPostingStub(t *testing.T) *postingStub {
	t.Helper()
	stub := newPostingStub(t)
	stub.fiscalYears = []any{
		map[string]any{
			"id": 45, "name": "2025", "open": true,
			"beginningTs": float64(1735686000000), "endTs": float64(1767141999999),
			"periods": []any{
				map[string]any{"name": "Juni 2025", "open": true, "beginningTs": float64(1748728800000), "endTs": float64(1751320799999)},
			},
		},
	}
	stub.sachkonten["4400"] = map[string]any{"id": 1, "number": "4400", "name": "Erlöse 19%", "active": true}
	stub.sachkonten["1200"] = map[string]any{"id": 2, "number": "1200", "name": "Bank", "active": true}
	stub.vatKeys = []string{"U19", "V19"}
	stub.postingResponse = map[string]any{"status": 201, "insertCount": "1", "updateCount": "0"}
	return stub
}

func stdoutStatus(t *testing.T, output string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output)
	}
	return got
}

func TestBuchungCreateDryRun(t *testing.T) {
	stub := newHappyPostingStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, stderr := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "dry_run" {
		t.Fatalf("stdout = %s", output.String())
	}
	for _, want := range []string{"POST /postings/new", "customer: 1234567", "SKR: skr04"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q in %q", want, stderr.String())
		}
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateDryRunAcceptsPersonalAccountsWithoutActiveField(t *testing.T) {
	cases := []struct {
		name   string
		number string
		add    func(*postingStub, string)
	}{
		{"debitor", "10001", func(stub *postingStub, number string) {
			stub.debitoren[number] = map[string]any{"number": number, "name": "Kunde", "contactId": 1}
		}},
		{"kreditor", "70001", func(stub *postingStub, number string) {
			stub.kreditoren[number] = map[string]any{"number": number, "name": "Lieferant", "contactId": 2}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newHappyPostingStub(t)
			stub.sachkonten["3300"] = map[string]any{"number": "3300", "name": "Sammelkonto", "active": true}
			tc.add(stub, tc.number)
			configPath := postingConfigPath(t, stub.server.URL)
			dataPath := postingFixture(t, postingInput([]any{
				map[string]any{"account": tc.number, "summaryAccount": "3300", "amount": 10.00},
				map[string]any{"account": "1200", "amount": -10.00},
			}))
			output, _ := withCLI(t, "", false)

			if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--dry-run"}); err != nil {
				t.Fatal(err)
			}
			if got := stdoutStatus(t, output.String()); got["status"] != "dry_run" {
				t.Fatalf("stdout = %s", output.String())
			}
			if stub.writeCount() != 0 {
				t.Fatalf("writes = %d", stub.writeCount())
			}
		})
	}
}

func TestBuchungCreateDryRunAcceptsNetRowsWithAutoCreateTax(t *testing.T) {
	stub := newHappyPostingStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, map[string]any{
		"documentNumber": "P-2025-1",
		"postingDate":    "2025-06-02",
		"autoCreateTax":  true,
		"rows": []any{
			map[string]any{"account": "4400", "amount": 100.00, "vatKey": "U19"},
			map[string]any{"account": "1200", "amount": -119.00},
		},
	})
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "dry_run" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func matchingJournalRows() []any {
	return matchingJournalRowsFor("P-2025-1")
}

func matchingJournalRowsFor(documentNumber string) []any {
	return []any{
		map[string]any{"documentNumber": documentNumber, "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 119.0, "creditAmount": 0.0, "vatKey": "U19"},
		map[string]any{"documentNumber": documentNumber, "postingDate": float64(1748822400000), "accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0},
	}
}

func postingInput(rows []any) map[string]any {
	return map[string]any{"documentNumber": "P-2025-1", "postingDate": "2025-06-02", "rows": rows}
}

func TestBuchungCreateRejectsSchemaInvalidWithoutRequests(t *testing.T) {
	stub := newHappyPostingStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, postingInput([]any{
		map[string]any{"account": "4400", "amount": 119.00},
	}))
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--dry-run"})
	if err == nil || !strings.Contains(err.Error(), "rows") {
		t.Fatalf("error = %v", err)
	}
	if len(stub.hits) != 0 {
		t.Fatalf("hits = %#v", stub.hits)
	}
}

func TestBuchungCreatePreflightFailures(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(stub *postingStub, input map[string]any)
		wantErr string
	}{
		{"closed period", func(stub *postingStub, _ map[string]any) {
			stub.fiscalYears = []any{map[string]any{
				"id": 45, "name": "2025", "open": true,
				"beginningTs": float64(1735686000000), "endTs": float64(1767141999999),
				"periods": []any{
					map[string]any{"name": "Juni 2025", "open": false, "beginningTs": float64(1748728800000), "endTs": float64(1751320799999)},
				},
			}}
		}, "is closed"},
		{"date outside years", func(_ *postingStub, input map[string]any) {
			input["postingDate"] = "2030-01-02"
		}, "no fiscal period contains postingDate 2030-01-02"},
		{"unknown account", func(_ *postingStub, input map[string]any) {
			input["rows"] = []any{
				map[string]any{"account": "9999", "amount": 119.00},
				map[string]any{"account": "1200", "amount": -119.00},
			}
		}, "account 9999 not found"},
		{"inactive account", func(stub *postingStub, _ map[string]any) {
			stub.sachkonten["4400"]["active"] = false
		}, "is inactive"},
		{"unknown vatKey", func(_ *postingStub, input map[string]any) {
			input["rows"] = []any{
				map[string]any{"account": "4400", "amount": 119.00, "vatKey": "X99"},
				map[string]any{"account": "1200", "amount": -119.00},
			}
		}, "vatKey X99 not found in Steuermatrix"},
		{"personenkonto without summaryAccount", func(stub *postingStub, input map[string]any) {
			stub.debitoren["70001"] = map[string]any{"number": "70001", "name": "Kunde", "active": true}
			input["rows"] = []any{
				map[string]any{"account": "70001", "amount": 119.00},
				map[string]any{"account": "1200", "amount": -119.00},
			}
		}, "summaryAccount is required for Personenkonto 70001"},
		{"unbalanced rows", func(_ *postingStub, input map[string]any) {
			input["rows"] = []any{
				map[string]any{"account": "4400", "amount": 119.00, "vatKey": "U19"},
				map[string]any{"account": "1200", "amount": -100.00},
			}
		}, "rows must balance to zero"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newHappyPostingStub(t)
			input := postingInput([]any{
				map[string]any{"account": "4400", "amount": 119.00, "vatKey": "U19"},
				map[string]any{"account": "1200", "amount": -119.00},
			})
			tc.mutate(stub, input)
			configPath := postingConfigPath(t, stub.server.URL)
			dataPath := postingFixture(t, input)
			withCLI(t, "", false)

			err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--dry-run"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
			if stub.writeCount() != 0 {
				t.Fatalf("writes = %d", stub.writeCount())
			}
		})
	}
}

func TestBuchungCreateRequiresTTYWithoutYes(t *testing.T) {
	stub := newHappyPostingStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateConfirmation(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.afterWrite = func() { stub.journalByID["P-2025-1"] = matchingJournalRows() }
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)

	withCLI(t, "yes\n", true)
	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath})
	if err == nil || !strings.Contains(err.Error(), "confirmation did not match") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}

	output, _ := withCLI(t, "create P-2025-1\n", true)
	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath}); err != nil {
		t.Fatal(err)
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "created" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestBuchungCreateYesHappyPath(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.afterWrite = func() { stub.journalByID["P-2025-1"] = matchingJournalRows() }
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
	body := stub.postingBodies[0]
	rows := body["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	for i, item := range rows {
		row := item.(map[string]any)
		if row["postingDate"] != float64(1748822400000) {
			t.Fatalf("rows[%d].postingDate = %#v", i, row["postingDate"])
		}
		if row["documentNumber"] != "P-2025-1" {
			t.Fatalf("rows[%d].documentNumber = %#v", i, row["documentNumber"])
		}
	}
	if rows[0].(map[string]any)["amount"] != float64(119) || rows[1].(map[string]any)["amount"] != float64(-119) {
		t.Fatalf("amounts = %#v", rows)
	}
	if stub.journalGetCount() != 2 {
		t.Fatalf("journal GETs = %d", stub.journalGetCount())
	}
	// Die Verifikation liest die vollständige Journalzeile ohne
	// fields-Projektion, damit personalAccountNumber mitkommt (scopeskill #58).
	for _, query := range stub.journalQueries {
		if query != "" {
			t.Fatalf("journal GET should fetch the full row, got query %q", query)
		}
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "created" {
		t.Fatalf("stdout = %s", output.String())
	}
	journal := got["journal"].(map[string]any)
	if len(journal["rows"].([]any)) != 2 {
		t.Fatalf("journal.rows = %#v", journal["rows"])
	}
}

func TestBuchungCreateUsesProviderAssignedDocumentNumber(t *testing.T) {
	stub := newHappyPostingStub(t)
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	stub.afterWrite = func() {
		stub.journalSearchRows = assignedRows
		stub.journalByID[assignedDocumentNumber] = assignedRows
	}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "created" || got["documentNumber"] != assignedDocumentNumber || got["requestedDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
	if len(stub.journalSearchBodies) != 2 {
		t.Fatalf("journal searches = %#v", stub.journalSearchBodies)
	}
	assignedReadBack := false
	for _, hit := range stub.hits {
		if hit == "GET /rest/journal/"+assignedDocumentNumber {
			assignedReadBack = true
		}
	}
	if !assignedReadBack {
		t.Fatalf("hits = %#v", stub.hits)
	}
}

func TestBuchungCreateFindsProviderAssignedDuplicate(t *testing.T) {
	stub := newHappyPostingStub(t)
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	stub.journalSearchRows = assignedRows
	stub.journalByID[assignedDocumentNumber] = assignedRows
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "already_exists" || got["documentNumber"] != assignedDocumentNumber || got["requestedDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateDoesNotWriteWhenAssignedDuplicateCannotBeReadBack(t *testing.T) {
	stub := newHappyPostingStub(t)
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	stub.journalSearchRows = assignedRows
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "could not be read back") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreatePaginatesAssignedDuplicateSearch(t *testing.T) {
	stub := newHappyPostingStub(t)
	firstPage := make([]any, 0, scopeskill.MaxSearchPageSize)
	for range scopeskill.MaxSearchPageSize {
		firstPage = append(firstPage, map[string]any{
			"documentNumber": "unrelated",
			"postingDate":    float64(1748822400000),
			"accountNumber":  "9999",
			"debitAmount":    1.0,
			"creditAmount":   0.0,
		})
	}
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	stub.journalSearchPages = map[int][]any{0: firstPage, 1: assignedRows}
	stub.journalByID[assignedDocumentNumber] = assignedRows
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "already_exists" || got["documentNumber"] != assignedDocumentNumber {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
	if len(stub.journalSearchBodies) != 2 {
		t.Fatalf("journal searches = %#v", stub.journalSearchBodies)
	}
}

func TestBuchungCreateIdenticalDuplicateAlreadyExists(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.journalByID["P-2025-1"] = matchingJournalRows()
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "already_exists" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateConflictingDuplicate(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.journalByID["P-2025-1"] = []any{
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 100.0, "creditAmount": 0.0, "vatKey": "U19"},
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0},
	}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "already exists with different rows") {
		t.Fatalf("error = %v", err)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "conflict" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 0 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateRejectedWriteSurfacesAPIError(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.postingStatus = 400
	stub.postingResponse = map[string]any{"error": "account 4400 blocked"}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "account 4400 blocked") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
	if stub.journalGetCount() != 1 {
		t.Fatalf("journal GETs = %d", stub.journalGetCount())
	}
}

func TestBuchungCreateAmbiguousResponseRequiresVerification(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.postingResponse = map[string]any{"status": 200, "insertCount": "0", "updateCount": "0"}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %v", err)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "verification_required" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateAcceptedWriteWithoutJournalRequiresVerification(t *testing.T) {
	stub := newHappyPostingStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "provider-assigned documentNumber not found") {
		t.Fatalf("error = %v", err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" || got["documentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if _, present := got["requestedDocumentNumber"]; present {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateDroppedConnectionVerifiesViaJournal(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.postingDropConnection = true
	stub.afterWrite = func() { stub.journalByID["P-2025-1"] = matchingJournalRows() }
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "created" || got["writeResponse"] != "ambiguous" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateDroppedConnectionUsesProviderAssignedDocumentNumber(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.postingDropConnection = true
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	stub.afterWrite = func() {
		stub.journalSearchRows = assignedRows
		stub.journalByID[assignedDocumentNumber] = assignedRows
	}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "created" || got["documentNumber"] != assignedDocumentNumber || got["requestedDocumentNumber"] != "P-2025-1" || got["writeResponse"] != "ambiguous" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}

func TestBuchungCreateVerificationMismatchFails(t *testing.T) {
	stub := newHappyPostingStub(t)
	stub.afterWrite = func() {
		stub.journalByID["P-2025-1"] = []any{
			map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 100.0, "creditAmount": 0.0, "vatKey": "U19"},
			map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0},
		}
	}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "verification_failed" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestBuchungCreateProviderAssignedVerificationMismatchReportsBothNumbers(t *testing.T) {
	stub := newHappyPostingStub(t)
	assignedDocumentNumber := "2026-000167"
	assignedRows := matchingJournalRowsFor(assignedDocumentNumber)
	mismatchedRows := matchingJournalRowsFor(assignedDocumentNumber)
	mismatchedRows[0].(map[string]any)["debitAmount"] = 100.0
	stub.afterWrite = func() {
		stub.journalSearchRows = assignedRows
		stub.journalByID[assignedDocumentNumber] = mismatchedRows
	}
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "create", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error = %v", err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_failed" || got["documentNumber"] != assignedDocumentNumber || got["requestedDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.writeCount() != 1 {
		t.Fatalf("writes = %d", stub.writeCount())
	}
}
