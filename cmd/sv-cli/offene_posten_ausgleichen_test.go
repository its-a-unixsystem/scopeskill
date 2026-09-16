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
)

type clearingCommandStub struct {
	server          *httptest.Server
	journal         map[string][]any
	openItems       map[string]map[string]any
	writeCount      int
	writeStatus     int
	ambiguousWrite  bool
	applyWrite      bool
	searchCount     int
	mutateOnSearch  int
	lastRequestBody map[string]any
}

func newClearingCommandStub(t *testing.T) *clearingCommandStub {
	t.Helper()
	stub := &clearingCommandStub{
		journal:     map[string][]any{},
		openItems:   map[string]map[string]any{},
		writeStatus: http.StatusOK,
		applyWrite:  true,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{"token_type": "Bearer", "access_token": "token", "expires_in": 3600})
		case r.URL.Path == "/rest/myaccount":
			writeJSONForCLI(w, map[string]any{"organisation": map[string]any{"id": 7, "name": "Unternehmen A"}})
		case strings.HasPrefix(r.URL.Path, "/rest/journal/"):
			documentNumber := strings.TrimPrefix(r.URL.Path, "/rest/journal/")
			rows, found := stub.journal[documentNumber]
			if !found {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			writeJSONForCLI(w, map[string]any{"records": rows})
		case r.URL.Path == "/rest/kreditoraccounts":
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			number := searchConditionValue(body, "number")
			records := []any{}
			if strings.HasPrefix(number, "7") {
				records = append(records, map[string]any{"number": number, "active": true})
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case r.URL.Path == "/rest/openitems/creditors":
			stub.searchCount++
			if stub.mutateOnSearch > 0 && stub.searchCount == stub.mutateOnSearch {
				stub.openItems["INV-1"]["amount"] = -118.0
			}
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			// Offene Posten sind nur über das Konto durchsuchbar; die
			// Belegnummer filtert der Aufrufer clientseitig (scopeskill #58).
			account := searchConditionValue(body, "accountNumber")
			records := []any{}
			for _, item := range stub.openItems {
				if account != "" && nonEmptyString(item["accountNumber"]) == account {
					records = append(records, item)
				}
			}
			writeJSONForCLI(w, map[string]any{"records": records})
		case r.URL.Path == "/rest/openitems/creditor/clearing":
			stub.writeCount++
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &stub.lastRequestBody)
			if stub.applyWrite {
				stub.applyClearing()
			}
			if stub.ambiguousWrite {
				hijacker := w.(http.Hijacker)
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
				return
			}
			if stub.writeStatus != http.StatusOK {
				http.Error(w, "provider rejected clearing", stub.writeStatus)
				return
			}
			writeJSONForCLI(w, map[string]any{"success": true})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func newHappyClearingCommandStub(t *testing.T) *clearingCommandStub {
	t.Helper()
	stub := newClearingCommandStub(t)
	stub.journal["PAY-1"] = creditorDocumentRows("PAY-1", "70010", 200, true)
	stub.journal["INV-1"] = creditorDocumentRows("INV-1", "70010", 119, false)
	stub.journal["INV-2"] = creditorDocumentRows("INV-2", "70010", 81, false)
	stub.openItems["PAY-1"] = openItem("PAY-1", "70010", 200, true)
	stub.openItems["INV-1"] = openItem("INV-1", "70010", 119, false)
	stub.openItems["INV-2"] = openItem("INV-2", "70010", 81, false)
	return stub
}

// creditorDocumentRows spiegelt das echte Journalformat: die Sammelkontozeile
// trägt accountNumber 3300 und den Kreditor unter personalAccountNumber, nicht
// die Kreditornummer im accountNumber, und eine Inlandsbuchung nennt keine
// Währung. Der Ausgleich muss den Kreditor dort erkennen (scopeskill #58).
func creditorDocumentRows(documentNumber, account string, amount float64, payment bool) []any {
	personalDebit, personalCredit := 0.0, amount
	counterDebit, counterCredit := amount, 0.0
	counterAccount := "4400"
	if payment {
		personalDebit, personalCredit = amount, 0
		counterDebit, counterCredit = 0, amount
		counterAccount = "1200"
	}
	return []any{
		map[string]any{"documentNumber": documentNumber, "accountNumber": "3300", "personalAccountNumber": account, "debitAmount": personalDebit, "creditAmount": personalCredit},
		map[string]any{"documentNumber": documentNumber, "accountNumber": counterAccount, "debitAmount": counterDebit, "creditAmount": counterCredit},
	}
}

// openItem spiegelt das echte OP-Format: die Belegnummer steht in
// postingNumber, der offene Betrag als vorzeichenbehaftetes amount (Rechnung
// negativ, Zahlung positiv), ohne eigenes openAmount oder currency.
func openItem(postingNumber, account string, open float64, payment bool) map[string]any {
	amount := -open
	if payment {
		amount = open
	}
	return map[string]any{
		"postingNumber": postingNumber,
		"accountNumber": account,
		"amount":        amount,
	}
}

func searchConditionValue(body map[string]any, field string) string {
	search, _ := body["search"].([]any)
	for _, value := range search {
		condition, _ := value.(map[string]any)
		if condition["field"] == field {
			return nonEmptyString(condition["value"])
		}
	}
	return ""
}

func (stub *clearingCommandStub) applyClearing() {
	clearings, _ := stub.lastRequestBody["clearings"].([]any)
	clearing, _ := clearings[0].(map[string]any)
	paymentNumber := nonEmptyString(clearing["documentNumber"])
	documents, _ := clearing["documents"].([]any)
	total := 0.0
	for _, value := range documents {
		document, _ := value.(map[string]any)
		number := nonEmptyString(document["documentNumber"])
		amount, _ := document["clearingAmount"].(float64)
		total += amount
		stub.reduceOpenAmount(number, amount)
	}
	stub.reduceOpenAmount(paymentNumber, total)
}

func (stub *clearingCommandStub) reduceOpenAmount(documentNumber string, amount float64) {
	item := stub.openItems[documentNumber]
	if item == nil {
		return
	}
	current := item["amount"].(float64)
	sign := 1.0
	if current < 0 {
		sign, current = -1.0, -current
	}
	remaining := current - amount
	if remaining == 0 {
		delete(stub.openItems, documentNumber)
		return
	}
	item["amount"] = sign * remaining
}

// openAmountOf gibt den Betrag eines offenen Postens als positiven Wert zurück
// (0, wenn ausgeglichen und damit nicht mehr gelistet).
func openAmountOf(stub *clearingCommandStub, postingNumber string) float64 {
	item := stub.openItems[postingNumber]
	if item == nil {
		return 0
	}
	amount, _ := item["amount"].(float64)
	if amount < 0 {
		return -amount
	}
	return amount
}

func clearingInputFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clearing.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func standardClearingInput(t *testing.T) string {
	t.Helper()
	return clearingInputFile(t, `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":119.00}]}`)
}

func runCreditorClear(t *testing.T, stub *clearingCommandStub, dataPath string, extra ...string) (map[string]any, string, error) {
	t.Helper()
	output, stderr := withCLI(t, "", false)
	args := []string{"--config", sachkontoConfigPath(t, stub.server.URL), "offene-posten", "clear", "--seite=kreditor", "--data", "@" + dataPath}
	args = append(args, extra...)
	err := run(args)
	var status map[string]any
	if output.Len() > 0 {
		status = stdoutStatus(t, output.String())
	}
	return status, stderr.String(), err
}

func TestOffenePostenClearDryRunPreflightsAndPreviews(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	status, stderr, err := runCreditorClear(t, stub, standardClearingInput(t), "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if stub.writeCount != 0 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
	if status["status"] != "dry_run" {
		t.Fatalf("stdout = %#v", status)
	}
	for _, want := range []string{"POST /openitems/creditor/clearing", "customer: 1234567", "Unternehmen A", "PAY-1", "INV-1", "119.00", "81.00"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q in %s", want, stderr)
		}
	}
}

func TestOffenePostenClearRequiresCreditorSide(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	withCLI(t, "", false)
	err := run([]string{"--config", sachkontoConfigPath(t, stub.server.URL), "offene-posten", "clear", "--seite=debitor", "--data", "@" + standardClearingInput(t), "--yes"})
	if err == nil || !strings.Contains(err.Error(), "--seite=kreditor") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount != 0 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
}

func TestOffenePostenClearRejectsMalformedInputBeforeRequests(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	path := clearingInputFile(t, `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":1.001}]}`)
	_, _, err := runCreditorClear(t, stub, path, "--yes")
	if err == nil || !strings.Contains(err.Error(), "two decimal") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount != 0 || stub.searchCount != 0 {
		t.Fatalf("write count = %d, search count = %d", stub.writeCount, stub.searchCount)
	}
}

func TestOffenePostenClearRejectsUnsafePreflightWithoutWrite(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*clearingCommandStub)
		input  string
		extra  []string
		want   string
	}{
		{"missing payment", func(s *clearingCommandStub) { delete(s.journal, "PAY-1") }, "", nil, "payment document"},
		{"closed item", func(s *clearingCommandStub) {
			delete(s.openItems, "INV-1")
			s.journal["INV-1"] = creditorDocumentRows("INV-1", "70010", 120, false)
		}, "", nil, "not open"},
		{"creditor mismatch", func(s *clearingCommandStub) {
			s.journal["INV-1"] = creditorDocumentRows("INV-1", "70011", 119, false)
			s.openItems["INV-1"]["accountNumber"] = "70011"
		}, "", nil, "same Kreditor"},
		{"currency mismatch", func(s *clearingCommandStub) {
			s.journal["INV-1"][0].(map[string]any)["foreignCurrencyCode"] = "USD"
		}, "", nil, "currency"},
		{"missing open amount", func(s *clearingCommandStub) { delete(s.openItems["INV-1"], "amount") }, "", nil, "lacks amount"},
		{"payment over-allocation", func(s *clearingCommandStub) { s.openItems["PAY-1"]["amount"] = 100.0 }, "", nil, "payment"},
		{"item over-clearing", func(s *clearingCommandStub) {}, `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":120.00}]}`, nil, "exceeds"},
		{"partial without flag", func(s *clearingCommandStub) {}, `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":100.00}]}`, nil, "--allow-partial"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := newHappyClearingCommandStub(t)
			tc.mutate(stub)
			path := standardClearingInput(t)
			if tc.input != "" {
				path = clearingInputFile(t, tc.input)
			}
			_, _, err := runCreditorClear(t, stub, path, append(tc.extra, "--yes")...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if stub.writeCount != 0 {
				t.Fatalf("write count = %d", stub.writeCount)
			}
		})
	}
}

func TestOffenePostenClearRequiresConfirmationBeforeWrite(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	_, _, err := runCreditorClear(t, stub, standardClearingInput(t))
	if err == nil || !strings.Contains(err.Error(), "TTY or --yes") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount != 0 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
}

func TestOffenePostenClearRequiresExactInteractivePhrase(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	dataPath := standardClearingInput(t)
	output, _ := withCLI(t, "clear PAY-1\n", true)
	err := run([]string{"--config", sachkontoConfigPath(t, stub.server.URL), "offene-posten", "clear", "--seite=kreditor", "--data", "@" + dataPath})
	if err != nil {
		t.Fatal(err)
	}
	if stub.writeCount != 1 || stdoutStatus(t, output.String())["status"] != "cleared" {
		t.Fatalf("write count = %d, stdout = %s", stub.writeCount, output.String())
	}
}

func TestOffenePostenClearWritesOnceAndVerifiesEveryBalance(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	path := clearingInputFile(t, `{"paymentDocumentNumber":"PAY-1","items":[{"documentNumber":"INV-1","clearingAmount":119.00},{"documentNumber":"INV-2","clearingAmount":50.00}]}`)
	status, _, err := runCreditorClear(t, stub, path, "--allow-partial", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if stub.writeCount != 1 || status["status"] != "cleared" {
		t.Fatalf("write count = %d, stdout = %#v", stub.writeCount, status)
	}
	if openAmountOf(stub, "PAY-1") != 31.0 || openAmountOf(stub, "INV-2") != 31.0 || stub.openItems["INV-1"] != nil {
		t.Fatalf("open items = %#v", stub.openItems)
	}
	items, _ := status["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("verified items = %#v", status["items"])
	}
}

func TestOffenePostenClearIdenticalRepeatMakesNoWrite(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	delete(stub.openItems, "INV-1")
	stub.openItems["PAY-1"]["amount"] = 81.0
	status, _, err := runCreditorClear(t, stub, standardClearingInput(t), "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "already_cleared" || stub.writeCount != 0 {
		t.Fatalf("write count = %d, stdout = %#v", stub.writeCount, status)
	}
}

func TestOffenePostenClearUsesExplicitBeforeStateForPreviouslyAllocatedItems(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	stub.openItems["PAY-1"]["amount"] = 150.0
	stub.openItems["INV-1"]["amount"] = -50.0
	path := clearingInputFile(t, `{"paymentDocumentNumber":"PAY-1","paymentOpenAmount":150.00,"items":[{"documentNumber":"INV-1","openAmount":50.00,"clearingAmount":50.00}]}`)

	status, _, err := runCreditorClear(t, stub, path, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "cleared" || stub.writeCount != 1 {
		t.Fatalf("write count = %d, stdout = %#v", stub.writeCount, status)
	}
	status, _, err = runCreditorClear(t, stub, path, "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "already_cleared" || stub.writeCount != 1 {
		t.Fatalf("write count = %d, stdout = %#v", stub.writeCount, status)
	}
}

func TestOffenePostenClearAbortsWhenLiveStateChangesBeforeWrite(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	stub.mutateOnSearch = 4 // payment + item preflight, then payment + changed item re-read
	_, _, err := runCreditorClear(t, stub, standardClearingInput(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "changed after preflight") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount != 0 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
}

func TestOffenePostenClearDoesNotRetryProviderRejection(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	stub.writeStatus = http.StatusBadRequest
	stub.applyWrite = false
	_, _, err := runCreditorClear(t, stub, standardClearingInput(t), "--yes")
	if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("error = %v", err)
	}
	if stub.writeCount != 1 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
}

func TestOffenePostenClearAmbiguousWriteReturnsVerificationRequiredWithoutRetry(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	stub.ambiguousWrite = true
	status, _, err := runCreditorClear(t, stub, standardClearingInput(t), "--yes")
	if err == nil || status["status"] != "verification_required" {
		t.Fatalf("error = %v, stdout = %#v", err, status)
	}
	if stub.writeCount != 1 || status["before"] == nil || status["lastObserved"] == nil {
		t.Fatalf("write count = %d, stdout = %#v", stub.writeCount, status)
	}
}

func TestOffenePostenClearPostWriteMismatchRequiresVerification(t *testing.T) {
	stub := newHappyClearingCommandStub(t)
	stub.applyWrite = false
	status, _, err := runCreditorClear(t, stub, standardClearingInput(t), "--yes")
	if err == nil || status["status"] != "verification_required" {
		t.Fatalf("error = %v, stdout = %#v", err, status)
	}
	if stub.writeCount != 1 {
		t.Fatalf("write count = %d", stub.writeCount)
	}
}
