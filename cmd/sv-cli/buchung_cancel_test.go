package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type cancelStub struct {
	server               *httptest.Server
	hits                 []string
	journalSearchBodies  []map[string]any
	cancelWrites         int
	journalByID          map[string][]any
	linkedRows           []any
	linkageStatus        int
	fiscalYears          any
	sachkonten           map[string]map[string]any
	debitoren            map[string]map[string]any
	kreditoren           map[string]map[string]any
	myaccount            any
	cancelStatus         int
	cancelResponse       any
	cancelDropConnection bool
	afterCancelWrite     func()
}

func newCancelStub(t *testing.T) *cancelStub {
	t.Helper()
	stub := &cancelStub{
		journalByID: map[string][]any{},
		linkedRows:  []any{},
		sachkonten:  map[string]map[string]any{},
		debitoren:   map[string]map[string]any{},
		kreditoren:  map[string]map[string]any{},
		myaccount:   map[string]any{"organisation": map[string]any{"id": 7, "name": "Unternehmen A", "extra": "ignored"}},
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
		case r.URL.Path == "/rest/myaccount" && r.Method == http.MethodGet:
			writeJSONForCLI(w, stub.myaccount)
		case r.URL.Path == "/rest/journal" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.journalSearchBodies = append(stub.journalSearchBodies, body)
			if stub.linkageStatus != 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(stub.linkageStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "linkage search failed"})
				return
			}
			writeJSONForCLI(w, map[string]any{"records": stub.linkedRows})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
			stub.cancelWrites++
			if stub.afterCancelWrite != nil {
				stub.afterCancelWrite()
			}
			if stub.cancelDropConnection {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack: %v", err)
					return
				}
				_ = conn.Close()
				return
			}
			status := stub.cancelStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if stub.cancelResponse != nil {
				_ = json.NewEncoder(w).Encode(stub.cancelResponse)
			}
		case strings.HasPrefix(r.URL.Path, "/rest/journal/") && r.Method == http.MethodGet:
			id := strings.TrimPrefix(r.URL.Path, "/rest/journal/")
			records, ok := stub.journalByID[id]
			if !ok {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func cancelOriginalRows() []any {
	return []any{
		map[string]any{
			"documentNumber": "P-2025-1", "postingDate": float64(1748822400000),
			"accountNumber": "4400", "debitAmount": 119.0, "creditAmount": 0.0,
			"vatKey": "U19", "postingText": "Wareneingang", "documentText": "Rechnung",
			"internalDocumentNumber": "INT-1", "externalDocumentNumber": "EXT-1",
			"documentDimension_1": 111.0,
		},
		map[string]any{
			"documentNumber": "P-2025-1", "postingDate": float64(1748822400000),
			"accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0,
			"postingText": "Wareneingang", "documentText": "Rechnung",
			"internalDocumentNumber": "INT-1", "externalDocumentNumber": "EXT-1",
		},
	}
}

func cancelStornoRows() []any {
	return []any{
		map[string]any{
			"documentNumber": "S-2025-1", "postingDate": float64(1748822400000),
			"accountNumber": "4400", "debitAmount": 0.0, "creditAmount": 119.0,
			"vatKey": "U19", "postingText": "Wareneingang", "documentText": "Rechnung",
			"internalDocumentNumber": "INT-1", "externalDocumentNumber": "EXT-1",
			"documentDimension_1": 111.0, "cancelDocument": "P-2025-1",
		},
		map[string]any{
			"documentNumber": "S-2025-1", "postingDate": float64(1748822400000),
			"accountNumber": "1200", "debitAmount": 119.0, "creditAmount": 0.0,
			"postingText": "Wareneingang", "documentText": "Rechnung",
			"internalDocumentNumber": "INT-1", "externalDocumentNumber": "EXT-1",
			"cancelDocument": "P-2025-1",
		},
	}
}

func cancelStornoLink() []any {
	return []any{
		map[string]any{"documentNumber": "S-2025-1", "cancelDocument": "P-2025-1"},
	}
}

func newHappyCancelStub(t *testing.T) *cancelStub {
	t.Helper()
	stub := newCancelStub(t)
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
	stub.journalByID["P-2025-1"] = cancelOriginalRows()
	stub.persistStorno()
	return stub
}

func (s *cancelStub) persistStorno() {
	s.afterCancelWrite = func() {
		s.linkedRows = cancelStornoLink()
		s.journalByID["S-2025-1"] = cancelStornoRows()
	}
}

func (s *cancelStub) journalGetCount(documentNumber string) int {
	count := 0
	for _, hit := range s.hits {
		if hit == "GET /rest/journal/"+documentNumber {
			count++
		}
	}
	return count
}

func TestBuchungCancelDryRun(t *testing.T) {
	stub := newHappyCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	output, stderr := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "dry_run" || got["state"] != "active" || got["originalDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	for _, want := range []string{"POST /journal/P-2025-1/cancel", "customer: 1234567", "Journal (Bearbeiten)", "Unternehmen A", "P-2025-1", "Wareneingang"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q in %q", want, stderr.String())
		}
	}
	for _, want := range []string{
		"GET /rest/journal/P-2025-1",
		"POST /rest/journal",
		"GET /rest/fiscalyears",
		"POST /rest/impersonalaccounts",
		"GET /rest/myaccount",
	} {
		found := false
		for _, hit := range stub.hits {
			if hit == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("hits missing %q: %#v", want, stub.hits)
		}
	}
	if stub.cancelWrites != 0 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelYesHappyPath(t *testing.T) {
	stub := newHappyCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"}); err != nil {
		t.Fatal(err)
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "cancelled" || got["state"] != "already_cancelled" {
		t.Fatalf("stdout = %s", output.String())
	}
	if got["originalDocumentNumber"] != "P-2025-1" || got["cancellationDocumentNumber"] != "S-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	cancellation, ok := got["cancellation"].(map[string]any)
	if !ok || len(cancellation["rows"].([]any)) != 2 {
		t.Fatalf("cancellation rows = %#v", got["cancellation"])
	}
	search := stub.journalSearchBodies[0]
	conditions := search["search"].([]any)
	cond := conditions[0].(map[string]any)
	if cond["field"] != "cancelDocument" || cond["operator"] != "equals" || cond["value"] != "P-2025-1" {
		t.Fatalf("linkage search = %#v", conditions)
	}
	fields, _ := search["fields"].([]any)
	found := false
	for _, field := range fields {
		if field == "cancelDocument" {
			found = true
		}
	}
	if !found {
		t.Fatalf("linkage fields missing cancelDocument: %#v", fields)
	}
	if stub.journalGetCount("P-2025-1") < 2 {
		t.Fatalf("original GETs = %d, want preflight + post-write read-back", stub.journalGetCount("P-2025-1"))
	}
}

func TestBuchungCancelAlreadyCancelled(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.linkedRows = cancelStornoLink()
	stub.journalByID["S-2025-1"] = cancelStornoRows()
	stub.afterCancelWrite = nil
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "already_cancelled" || got["state"] != "already_cancelled" {
		t.Fatalf("stdout = %s", output.String())
	}
	if got["cancellationDocumentNumber"] != "S-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 0 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelZeroWriteConflicts(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(stub *cancelStub)
		wantStatus string
		wantErr    string
	}{
		{"missing original", func(stub *cancelStub) {
			delete(stub.journalByID, "P-2025-1")
		}, "conflict", "not found"},
		{"target is a cancellation", func(stub *cancelStub) {
			for _, item := range stub.journalByID["P-2025-1"] {
				item.(map[string]any)["cancelDocument"] = "X-2025-9"
			}
		}, "conflict", "cancellation document"},
		{"closed period", func(stub *cancelStub) {
			stub.fiscalYears = []any{map[string]any{
				"id": 45, "name": "2025", "open": true,
				"beginningTs": float64(1735686000000), "endTs": float64(1767141999999),
				"periods": []any{
					map[string]any{"name": "Juni 2025", "open": false, "beginningTs": float64(1748728800000), "endTs": float64(1751320799999)},
				},
			}}
		}, "conflict", "is closed"},
		{"inactive account", func(stub *cancelStub) {
			stub.sachkonten["4400"]["active"] = false
		}, "conflict", "is inactive"},
		{"missing account", func(stub *cancelStub) {
			delete(stub.sachkonten, "4400")
		}, "conflict", "not found"},
		{"inconsistent original", func(stub *cancelStub) {
			stub.journalByID["P-2025-1"][1].(map[string]any)["documentText"] = "anders"
		}, "conflict", "inconsistent"},
		{"linked candidate not a Storno", func(stub *cancelStub) {
			stub.linkedRows = []any{map[string]any{"documentNumber": "R-2025-1", "cancelDocument": "P-2025-1"}}
			stub.journalByID["R-2025-1"] = []any{
				map[string]any{"documentNumber": "R-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 50.0, "creditAmount": 0.0, "cancelDocument": "P-2025-1"},
			}
		}, "conflict", "Storno"},
		{"linkage search failure", func(stub *cancelStub) {
			stub.linkageStatus = 500
		}, "verification_required", "linkage search failed"},
		{"linkage row without cancelDocument", func(stub *cancelStub) {
			stub.linkedRows = []any{map[string]any{"documentNumber": "R-2025-1"}}
		}, "verification_required", ""},
		{"linked document rows without cancelDocument", func(stub *cancelStub) {
			stub.linkedRows = cancelStornoLink()
			stub.journalByID["S-2025-1"] = []any{
				map[string]any{"documentNumber": "S-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 0.0, "creditAmount": 119.0},
			}
		}, "verification_required", "cancelDocument"},
		{"malformed myaccount", func(stub *cancelStub) {
			stub.myaccount = map[string]any{"username": "someone"}
		}, "verification_required", "organisation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newHappyCancelStub(t)
			tc.mutate(stub)
			configPath := postingConfigPath(t, stub.server.URL)
			output, _ := withCLI(t, "", false)

			err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
			got := stdoutStatus(t, output.String())
			if got["status"] != tc.wantStatus || got["state"] != tc.wantStatus {
				t.Fatalf("stdout = %s", output.String())
			}
			if stub.cancelWrites != 0 {
				t.Fatalf("cancel writes = %d", stub.cancelWrites)
			}
		})
	}
}

func TestBuchungCancelLinkageFailureClaimsNoVerifiedState(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.linkageStatus = 500
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" {
		t.Fatalf("stdout = %s", output.String())
	}
	if _, present := got["lastVerifiedState"]; present {
		t.Fatalf("stdout unexpectedly claims a verified state: %s", output.String())
	}
	if stub.cancelWrites != 0 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelConfirmation(t *testing.T) {
	stub := newHappyCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)

	output, _ := withCLI(t, "", false)
	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1"})
	if err == nil || !strings.Contains(err.Error(), "requires a TTY") {
		t.Fatalf("error = %v", err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "conflict" || got["state"] != "active" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 0 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}

	output, _ = withCLI(t, "yes\n", true)
	err = run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1"})
	if err == nil || !strings.Contains(err.Error(), "confirmation did not match") {
		t.Fatalf("error = %v", err)
	}
	got = stdoutStatus(t, output.String())
	if got["status"] != "conflict" || got["state"] != "active" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 0 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}

	output, _ = withCLI(t, "cancel P-2025-1\n", true)
	if err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1"}); err != nil {
		t.Fatal(err)
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "cancelled" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestBuchungCancelRejectedWriteSurfacesAPIError(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.cancelStatus = 400
	stub.cancelResponse = map[string]any{"error": "posting period is closed"}
	stub.afterCancelWrite = nil
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "posting period is closed") {
		t.Fatalf("error = %v", err)
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
	if got := stdoutStatus(t, output.String()); got["status"] != "conflict" {
		t.Fatalf("stdout = %s", output.String())
	}
}

func TestBuchungCancelDroppedConnectionVerifiesViaJournal(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.cancelDropConnection = true
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"}); err != nil {
		t.Fatal(err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "cancelled" || got["writeResponse"] != "ambiguous" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelDroppedConnectionNotPersisted(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.cancelDropConnection = true
	stub.afterCancelWrite = nil
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "write error") {
		t.Fatalf("error missing write diagnostic: %v", err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" || got["originalDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelAmbiguousOKWithoutStorno(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.cancelResponse = map[string]any{"cancellationDocumentNumber": "S-2025-9"}
	stub.afterCancelWrite = nil
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" || got["originalDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if got["cancellationDocumentNumber"] != "S-2025-9" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelPostWriteOriginalChanged(t *testing.T) {
	stub := newHappyCancelStub(t)
	original := stub.afterCancelWrite
	stub.afterCancelWrite = func() {
		original()
		delete(stub.journalByID, "P-2025-1")
	}
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" || got["cancellationDocumentNumber"] != "S-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelPostWriteOriginalMutated(t *testing.T) {
	stub := newHappyCancelStub(t)
	original := stub.afterCancelWrite
	stub.afterCancelWrite = func() {
		original()
		stub.journalByID["P-2025-1"][0].(map[string]any)["debitAmount"] = 100.0
	}
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungCancelReadBackMismatchRequiresVerification(t *testing.T) {
	stub := newHappyCancelStub(t)
	stub.afterCancelWrite = func() {
		stub.linkedRows = []any{map[string]any{"documentNumber": "S-2025-1", "cancelDocument": "P-2025-1"}}
		stub.journalByID["S-2025-1"] = []any{
			map[string]any{"documentNumber": "S-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 0.0, "creditAmount": 100.0, "cancelDocument": "P-2025-1"},
		}
	}
	configPath := postingConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "cancel", "P-2025-1", "--yes"})
	if err == nil {
		t.Fatal("expected error")
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "verification_required" {
		t.Fatalf("stdout = %s", output.String())
	}
	if stub.cancelWrites != 1 {
		t.Fatalf("cancel writes = %d", stub.cancelWrites)
	}
}

func TestBuchungReplaceIsGated(t *testing.T) {
	stub := newCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)
	output, _ := withCLI(t, "", false)

	err := run([]string{"--config", configPath, "buchung", "replace", "P-2025-1", "--data", "@" + dataPath, "--yes"})
	if err == nil || !strings.Contains(err.Error(), "/postings/correction") || !strings.Contains(err.Error(), "buchung cancel") {
		t.Fatalf("error = %v", err)
	}
	got := stdoutStatus(t, output.String())
	if got["status"] != "conflict" || got["state"] != "conflict" || got["originalDocumentNumber"] != "P-2025-1" {
		t.Fatalf("stdout = %s", output.String())
	}
	if len(stub.hits) != 0 {
		t.Fatalf("hits = %#v", stub.hits)
	}
}

func TestBuchungReplaceSyntaxValidationMakesNoRequests(t *testing.T) {
	stub := newCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	dataPath := postingFixture(t, validPostingInput)

	for _, args := range [][]string{
		{"buchung", "replace", "P-2025-1"},
		{"buchung", "replace", "P-*", "--data", "@" + dataPath},
		{"buchung", "replace", "P-2025-1", "Q-2025-2", "--data", "@" + dataPath},
	} {
		withCLI(t, "", false)
		if err := run(append([]string{"--config", configPath}, args...)); err == nil {
			t.Fatalf("args %v: expected error", args)
		}
	}
	if len(stub.hits) != 0 {
		t.Fatalf("hits = %#v", stub.hits)
	}
}

func TestBuchungCancelRejectsWildcardWithoutRequests(t *testing.T) {
	stub := newCancelStub(t)
	configPath := postingConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	for _, target := range []string{"P-*", "P-2025-?"} {
		if err := run([]string{"--config", configPath, "buchung", "cancel", target, "--yes"}); err == nil {
			t.Fatalf("target %q: expected error", target)
		}
	}
	if len(stub.hits) != 0 {
		t.Fatalf("hits = %#v", stub.hits)
	}
}
