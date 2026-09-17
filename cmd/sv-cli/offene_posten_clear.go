package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const offenePostenClearUsage = `usage: sv-cli offene-posten clear --seite=debitor|kreditor --data @clearing.json [--dry-run] [--allow-partial] [--yes]

Input schema (--data JSON or @file):
  paymentDocumentNumber     required payment Belegnummer
  paymentOpenAmount         optional expected before-state; requires every item openAmount
  items                     required; at least one selected open item
  items[].documentNumber    required open-item Belegnummer
  items[].openAmount        optional expected before-state; requires paymentOpenAmount
  items[].clearingAmount    required positive EUR amount with at most two decimals

Safety:
  --dry-run       perform read-only preflight and preview; no clearing write
  --allow-partial permit an item clearingAmount below its current open amount
  --yes           bypass the interactive "clear <paymentDocumentNumber>" confirmation

The command requires --seite=debitor|kreditor, sends at most one clearing request,
never retries a write, and verifies every affected balance before reporting success.`

type centAmount int64

func (amount centAmount) MarshalJSON() ([]byte, error) {
	cents := int64(amount)
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return []byte(fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)), nil
}

func (amount *centAmount) UnmarshalJSON(raw []byte) error {
	if len(raw) == 0 || raw[0] == '"' {
		return errors.New("clearingAmount must be a JSON number")
	}
	parsed, err := parseDecimalCents(string(raw))
	if err != nil {
		return err
	}
	*amount = parsed
	return nil
}

func parseDecimalCents(raw string) (centAmount, error) {
	if raw == "" || strings.ContainsAny(raw, "eE+") {
		return 0, errors.New("clearingAmount must be a decimal number with at most two decimal places")
	}
	negative := strings.HasPrefix(raw, "-")
	unsigned := strings.TrimPrefix(raw, "-")
	parts := strings.Split(unsigned, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 2)) {
		return 0, errors.New("clearingAmount must have at most two decimal places")
	}
	for _, part := range parts {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return 0, errors.New("clearingAmount must be a decimal number")
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > (int64(^uint64(0)>>1)-99)/100 {
		return 0, errors.New("clearingAmount is out of range")
	}
	fraction := int64(0)
	if len(parts) == 2 {
		fractionText := parts[1]
		if len(fractionText) == 1 {
			fractionText += "0"
		}
		fraction, _ = strconv.ParseInt(fractionText, 10, 64)
	}
	cents := whole*100 + fraction
	if negative {
		cents = -cents
	}
	return centAmount(cents), nil
}

type creditorClearingInput struct {
	PaymentDocumentNumber string                      `json:"paymentDocumentNumber"`
	PaymentOpenAmount     *centAmount                 `json:"paymentOpenAmount,omitempty"`
	Items                 []creditorClearingInputItem `json:"items"`
}

type creditorClearingInputItem struct {
	DocumentNumber string      `json:"documentNumber"`
	OpenAmount     *centAmount `json:"openAmount,omitempty"`
	ClearingAmount centAmount  `json:"clearingAmount"`
}

type openItemsClearingRequest struct {
	Clearings []openItemsClearing `json:"clearings"`
}

type openItemsClearing struct {
	DocumentNumber string                      `json:"documentNumber"`
	Documents      []openItemsClearingDocument `json:"documents"`
}

type openItemsClearingDocument struct {
	DocumentNumber string     `json:"documentNumber"`
	ClearingAmount centAmount `json:"clearingAmount"`
}

type creditorClearingDocument struct {
	DocumentNumber string
	AccountNumber  string
	OriginalAmount centAmount
}

type creditorClearingBalance struct {
	DocumentNumber string     `json:"documentNumber"`
	Before         centAmount `json:"beforeOpenAmount"`
	Allocation     centAmount `json:"clearingAmount"`
	After          centAmount `json:"afterOpenAmount"`
}

type creditorClearingSnapshot struct {
	Payment creditorClearingBalance   `json:"payment"`
	Items   []creditorClearingBalance `json:"items"`
}

func parseCreditorClearingInput(raw []byte) (creditorClearingInput, error) {
	var input creditorClearingInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return creditorClearingInput{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return creditorClearingInput{}, errors.New("unexpected trailing data after JSON document")
	}
	input.PaymentDocumentNumber = strings.TrimSpace(input.PaymentDocumentNumber)
	if input.PaymentDocumentNumber == "" {
		return creditorClearingInput{}, errors.New("paymentDocumentNumber is required")
	}
	if len(input.Items) == 0 {
		return creditorClearingInput{}, errors.New("items must contain at least one entry")
	}
	seen := make(map[string]struct{}, len(input.Items))
	for i := range input.Items {
		item := &input.Items[i]
		item.DocumentNumber = strings.TrimSpace(item.DocumentNumber)
		switch {
		case item.DocumentNumber == "":
			return creditorClearingInput{}, fmt.Errorf("items[%d]: documentNumber is required", i+1)
		case item.DocumentNumber == input.PaymentDocumentNumber:
			return creditorClearingInput{}, fmt.Errorf("items[%d]: payment document cannot also be a clearing item", i+1)
		case item.ClearingAmount <= 0:
			return creditorClearingInput{}, fmt.Errorf("items[%d]: clearingAmount must be positive", i+1)
		}
		if _, duplicate := seen[item.DocumentNumber]; duplicate {
			return creditorClearingInput{}, fmt.Errorf("items[%d]: duplicate documentNumber %q", i+1, item.DocumentNumber)
		}
		seen[item.DocumentNumber] = struct{}{}
	}
	if input.PaymentOpenAmount != nil && *input.PaymentOpenAmount <= 0 {
		return creditorClearingInput{}, errors.New("paymentOpenAmount must be positive")
	}
	hasRequestedBeforeState := input.PaymentOpenAmount != nil
	for i, item := range input.Items {
		if item.OpenAmount != nil && *item.OpenAmount <= 0 {
			return creditorClearingInput{}, fmt.Errorf("items[%d]: openAmount must be positive", i+1)
		}
		if (item.OpenAmount != nil) != hasRequestedBeforeState {
			return creditorClearingInput{}, errors.New("paymentOpenAmount and every items[].openAmount must be supplied together")
		}
	}
	return input, nil
}

func (input creditorClearingInput) providerRequest() openItemsClearingRequest {
	documents := make([]openItemsClearingDocument, len(input.Items))
	for i, item := range input.Items {
		documents[i] = openItemsClearingDocument{DocumentNumber: item.DocumentNumber, ClearingAmount: item.ClearingAmount}
	}
	return openItemsClearingRequest{Clearings: []openItemsClearing{{DocumentNumber: input.PaymentDocumentNumber, Documents: documents}}}
}

func offenePostenClear(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("offene-posten clear", flag.ContinueOnError)
	flags.SetOutput(cliError)
	seiteFlag := flags.String("seite", "", "required OP side: debitor or kreditor")
	data := flags.String("data", "", "clearing JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preflight and preview only; no write")
	allowPartial := flags.Bool("allow-partial", false, "allow partial clearing of selected items")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, offenePostenClearUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *data == "" {
		flags.Usage()
		return errors.New("offene-posten clear requires --data and takes no positional arguments")
	}
	seite, err := parseOffenePostenSeite(*seiteFlag)
	if err != nil {
		return err
	}
	raw, err := loadRaw(*data)
	if err != nil {
		return err
	}
	input, err := parseCreditorClearingInput(raw)
	if err != nil {
		return err
	}

	documents, err := fetchClearingDocuments(client, seite, input)
	if err != nil {
		return err
	}
	before, disposition, err := preflightClearing(client, seite, input, documents, *allowPartial)
	if err != nil {
		return err
	}
	organisation, err := fetchClearingOrganisation(client)
	if err != nil {
		return err
	}
	requestBody := input.providerRequest()
	req := writeRequest{
		Command:       "offene-posten clear",
		Method:        http.MethodPost,
		Path:          seite.clearingEndpoint,
		Payload:       requestBody,
		ConfirmPhrase: "clear " + input.PaymentDocumentNumber,
		Preview: map[string]any{
			"requiredProfile": "Journal (Bearbeiten)",
			"organisation":    organisation,
			"allocation":      before,
			"request":         requestBody,
		},
	}
	writePreview(client, req)

	baseOutput := map[string]any{
		"endpoint":     "POST " + req.Path,
		"organisation": organisation,
		"request":      requestBody,
		"payment":      before.Payment,
		"items":        before.Items,
	}
	if disposition == clearingAlreadyApplied {
		baseOutput["status"] = "already_cleared"
		return printJSON(baseOutput)
	}
	if *dryRun {
		baseOutput["status"] = "dry_run"
		return printJSON(baseOutput)
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	liveBefore, err := readClearingSnapshot(client, seite, input, documents)
	if err != nil {
		return err
	}
	if !sameClearingOpenAmounts(before, liveBefore) {
		return errors.New("open-item balances changed after preflight; no clearing write performed")
	}

	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome == writeRejected {
		return writeErr
	}
	lastObserved, observeErr := readClearingSnapshot(client, seite, input, documents)
	if outcome == writeAmbiguous {
		return clearingVerificationRequired(input, before, lastObserved, result, writeErr, observeErr)
	}
	if observeErr != nil {
		return clearingVerificationRequired(input, before, lastObserved, result, nil, observeErr)
	}
	if !matchesPredictedClearing(before, lastObserved) {
		return clearingVerificationRequired(input, before, lastObserved, result, nil, errors.New("post-write balances do not match the requested allocation"))
	}
	baseOutput["status"] = "cleared"
	baseOutput["response"] = result
	baseOutput["payment"] = verifiedBalance(before.Payment, lastObserved.Payment)
	baseOutput["items"] = verifiedBalances(before.Items, lastObserved.Items)
	return printJSON(baseOutput)
}

type clearingDisposition int

const (
	clearingReady clearingDisposition = iota
	clearingAlreadyApplied
)

func fetchClearingDocuments(client *scopeskill.Client, seite offenePostenSeite, input creditorClearingInput) (map[string]creditorClearingDocument, error) {
	numbers := clearingDocumentNumbers(input)
	netsByDocument := make(map[string]map[string]centAmount, len(numbers))
	currencyByDocument := make(map[string]map[string]string, len(numbers))
	for i, number := range numbers {
		records, err := fetchJournalRecords(client, number)
		if err != nil {
			return nil, err
		}
		if len(records) == 0 {
			if i == 0 {
				return nil, fmt.Errorf("payment document %s was not found", number)
			}
			return nil, fmt.Errorf("selected item document %s was not found", number)
		}
		nets, currencies, err := journalAccountNets(records)
		if err != nil {
			return nil, fmt.Errorf("journal document %s: %w", number, err)
		}
		netsByDocument[number] = nets
		currencyByDocument[number] = currencies
	}

	var common []string
	for account, amount := range netsByDocument[numbers[0]] {
		if amount == 0 {
			continue
		}
		shared := true
		for _, number := range numbers[1:] {
			if netsByDocument[number][account] == 0 {
				shared = false
				break
			}
		}
		if shared {
			common = append(common, account)
		}
	}
	var personalAccounts []string
	for _, candidate := range common {
		account, err := fetchPersonalAccountByNumber(client, seite.accountKind, candidate)
		if err != nil {
			return nil, err
		}
		if account != nil {
			personalAccounts = append(personalAccounts, candidate)
		}
	}
	accountLabel := "Kreditor"
	if seite.name == "debitor" {
		accountLabel = "Debitor"
	}
	if len(personalAccounts) != 1 {
		return nil, fmt.Errorf("payment and selected items must belong to the same %s account", accountLabel)
	}
	account := personalAccounts[0]
	documents := make(map[string]creditorClearingDocument, len(numbers))
	paymentDirection := signOf(netsByDocument[numbers[0]][account])
	// Inlandsbuchungen führen keine Währung auf der Zeile; das ist EUR. Eine
	// ausgewiesene Fremdwährung bleibt dagegen vom Ausgleich ausgeschlossen.
	currency := clearingCurrency(currencyByDocument[numbers[0]][account])
	if currency != "EUR" {
		return nil, fmt.Errorf("payment currency %s is not supported; %s clearing requires EUR", currency, seite.name)
	}
	for i, number := range numbers {
		net := netsByDocument[number][account]
		documentCurrency := clearingCurrency(currencyByDocument[number][account])
		if documentCurrency != currency {
			return nil, fmt.Errorf("document %s currency %s does not match payment currency %s", number, documentCurrency, currency)
		}
		if i > 0 && signOf(net) == paymentDirection {
			return nil, fmt.Errorf("selected item %s is not opposite the payment on %s account %s", number, accountLabel, account)
		}
		documents[number] = creditorClearingDocument{
			DocumentNumber: number,
			AccountNumber:  account,
			OriginalAmount: absCents(net),
		}
	}
	return documents, nil
}

func journalAccountNets(records []any) (map[string]centAmount, map[string]string, error) {
	nets := map[string]centAmount{}
	currencies := map[string]string{}
	for _, value := range records {
		record, _ := value.(map[string]any)
		// Auf der Sammelkontozeile trägt Scopevisio das Personenkonto unter
		// personalAccountNumber; accountNumber zeigt das Sammelkonto. Der
		// Ausgleich läuft über das Personenkonto (scopeskill #58).
		account := firstNonEmptyString(record, "personalAccountNumber", "accountNumber")
		if account == "" {
			continue
		}
		debit, err := anyAmountCents(record["debitAmount"])
		if err != nil {
			return nil, nil, err
		}
		credit, err := anyAmountCents(record["creditAmount"])
		if err != nil {
			return nil, nil, err
		}
		nets[account] += debit - credit
		rowCurrency := firstNonEmptyString(record, "foreignCurrencyCode", "currencyCode", "currency")
		if rowCurrency != "" {
			currencies[account] = rowCurrency
		}
	}
	return nets, currencies, nil
}

func preflightClearing(client *scopeskill.Client, seite offenePostenSeite, input creditorClearingInput, documents map[string]creditorClearingDocument, allowPartial bool) (creditorClearingSnapshot, clearingDisposition, error) {
	snapshot, err := readClearingSnapshot(client, seite, input, documents)
	if err != nil {
		return creditorClearingSnapshot{}, clearingReady, err
	}
	total := snapshot.Payment.Allocation
	paymentBefore := snapshot.Payment.After + total
	ready := snapshot.Payment.Before == paymentBefore
	already := snapshot.Payment.Before == snapshot.Payment.After
	for i, balance := range snapshot.Items {
		requestedBefore := balance.After + balance.Allocation
		if balance.Allocation > requestedBefore {
			return creditorClearingSnapshot{}, clearingReady, fmt.Errorf("items[%d]: clearingAmount exceeds the requested before-state", i+1)
		}
		if balance.Before == 0 && !already {
			return creditorClearingSnapshot{}, clearingReady, fmt.Errorf("selected item %s is closed or not open at the requested before-state", balance.DocumentNumber)
		}
		if balance.Before < balance.Allocation && !already {
			return creditorClearingSnapshot{}, clearingReady, fmt.Errorf("items[%d]: clearingAmount exceeds the current open amount", i+1)
		}
		if requestedBefore > balance.Allocation && !allowPartial {
			return creditorClearingSnapshot{}, clearingReady, fmt.Errorf("items[%d]: partial clearing requires --allow-partial", i+1)
		}
		ready = ready && balance.Before == requestedBefore
		already = already && balance.Before == balance.After
	}
	if snapshot.Payment.Before < total && !already {
		return creditorClearingSnapshot{}, clearingReady, errors.New("sum of clearing amounts exceeds the unallocated payment amount")
	}
	if already {
		return snapshot, clearingAlreadyApplied, nil
	}
	if !ready {
		return creditorClearingSnapshot{}, clearingReady, errors.New("live balances do not match the requested before-state or this exact completed clearing")
	}
	return snapshot, clearingReady, nil
}

func readClearingSnapshot(client *scopeskill.Client, seite offenePostenSeite, input creditorClearingInput, documents map[string]creditorClearingDocument) (creditorClearingSnapshot, error) {
	total := centAmount(0)
	for _, item := range input.Items {
		total += item.ClearingAmount
	}
	paymentOpen, err := fetchClearingOpenAmount(client, seite, documents[input.PaymentDocumentNumber])
	if err != nil {
		return creditorClearingSnapshot{}, err
	}
	paymentBefore := documents[input.PaymentDocumentNumber].OriginalAmount
	if input.PaymentOpenAmount != nil {
		paymentBefore = *input.PaymentOpenAmount
	}
	snapshot := creditorClearingSnapshot{Payment: creditorClearingBalance{
		DocumentNumber: input.PaymentDocumentNumber,
		Before:         paymentOpen,
		Allocation:     total,
		After:          paymentBefore - total,
	}}
	if snapshot.Payment.After < 0 {
		return creditorClearingSnapshot{}, errors.New("sum of clearing amounts exceeds the payment document amount")
	}
	for _, item := range input.Items {
		document := documents[item.DocumentNumber]
		open, err := fetchClearingOpenAmount(client, seite, document)
		if err != nil {
			return creditorClearingSnapshot{}, err
		}
		itemBefore := document.OriginalAmount
		if item.OpenAmount != nil {
			itemBefore = *item.OpenAmount
		}
		snapshot.Items = append(snapshot.Items, creditorClearingBalance{
			DocumentNumber: item.DocumentNumber,
			Before:         open,
			Allocation:     item.ClearingAmount,
			After:          itemBefore - item.ClearingAmount,
		})
	}
	return snapshot, nil
}

func fetchClearingOpenAmount(client *scopeskill.Client, seite offenePostenSeite, document creditorClearingDocument) (centAmount, error) {
	// Offene Posten lassen sich nur über das Konto suchen: postingNumber ist
	// kein durchsuchbares Feld (die API antwortet mit HTTP 500), und die
	// Belegnummer steht dort, nicht unter invoiceNumber. Deshalb alle offenen
	// Posten des Personenkontos holen und über die Belegnummer filtern
	// (scopeskill #58). Ist keiner mehr offen, ist der Posten ausgeglichen.
	base := scopeskill.SearchRequest{
		Conditions: []scopeskill.SearchCondition{{Field: "accountNumber", Operator: scopeskill.OpEquals, Value: document.AccountNumber}},
	}
	records, err := paginateOpenItems(client, seite.endpoint, base, true, 0, 0)
	if err != nil {
		return 0, err
	}
	var match map[string]any
	for _, value := range records {
		record, _ := value.(map[string]any)
		if nonEmptyString(record["postingNumber"]) != document.DocumentNumber {
			continue
		}
		if match != nil {
			return 0, fmt.Errorf("document %s matched multiple %s open items", document.DocumentNumber, seite.name)
		}
		match = record
	}
	if match == nil {
		return 0, nil
	}
	// Der offene Betrag steht als vorzeichenbehaftetes amount; ein eigenes
	// openAmount führt Scopevisio hier nicht. Ob es der Rest- oder der
	// Ursprungsbetrag ist, ist für Teilausgleiche noch zu klären
	// (docs/scopeskill-luecken.md).
	amountValue, present := match["amount"]
	if !present {
		return 0, fmt.Errorf("document %s open item lacks amount", document.DocumentNumber)
	}
	open, err := anyAmountCents(amountValue)
	if err != nil {
		return 0, fmt.Errorf("document %s amount: %w", document.DocumentNumber, err)
	}
	return absCents(open), nil
}

func clearingCurrency(raw string) string {
	if currency := normalizeCurrency(raw); currency != "" {
		return currency
	}
	return "EUR"
}

func fetchClearingOrganisation(client *scopeskill.Client) (map[string]any, error) {
	raw, err := client.JSON(http.MethodGet, "/myaccount", nil, nil)
	if err != nil {
		return nil, err
	}
	account, _ := raw.(map[string]any)
	organisation, _ := account["organisation"].(map[string]any)
	if organisation == nil {
		return nil, errors.New("/myaccount response lacks the organisation object")
	}
	return map[string]any{"id": organisation["id"], "name": organisation["name"]}, nil
}

func anyAmountCents(value any) (centAmount, error) {
	var raw string
	switch number := value.(type) {
	case nil:
		raw = "0"
	case json.Number:
		raw = number.String()
	case float64:
		raw = strconv.FormatFloat(number, 'f', -1, 64)
	case string:
		raw = number
	default:
		return 0, fmt.Errorf("amount has unsupported type %T", value)
	}
	return parseDecimalCents(raw)
}

func firstNonEmptyString(record map[string]any, fields ...string) string {
	for _, field := range fields {
		if value := nonEmptyString(record[field]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

func signOf(amount centAmount) int {
	if amount < 0 {
		return -1
	}
	if amount > 0 {
		return 1
	}
	return 0
}

func absCents(amount centAmount) centAmount {
	if amount < 0 {
		return -amount
	}
	return amount
}

func sameClearingOpenAmounts(left, right creditorClearingSnapshot) bool {
	if left.Payment.Before != right.Payment.Before || len(left.Items) != len(right.Items) {
		return false
	}
	for i := range left.Items {
		if left.Items[i].DocumentNumber != right.Items[i].DocumentNumber || left.Items[i].Before != right.Items[i].Before {
			return false
		}
	}
	return true
}

func matchesPredictedClearing(before, observed creditorClearingSnapshot) bool {
	if observed.Payment.Before != before.Payment.Before-before.Payment.Allocation || len(before.Items) != len(observed.Items) {
		return false
	}
	for i := range before.Items {
		if observed.Items[i].DocumentNumber != before.Items[i].DocumentNumber || observed.Items[i].Before != before.Items[i].Before-before.Items[i].Allocation {
			return false
		}
	}
	return true
}

func verifiedBalance(before, observed creditorClearingBalance) creditorClearingBalance {
	before.After = observed.Before
	return before
}

func verifiedBalances(before, observed []creditorClearingBalance) []creditorClearingBalance {
	verified := make([]creditorClearingBalance, len(before))
	for i := range before {
		verified[i] = verifiedBalance(before[i], observed[i])
	}
	return verified
}

func clearingVerificationRequired(input creditorClearingInput, before, observed creditorClearingSnapshot, result any, writeErr, observeErr error) error {
	output := map[string]any{
		"status":                "verification_required",
		"paymentDocumentNumber": input.PaymentDocumentNumber,
		"documentNumbers":       clearingDocumentNumbers(input),
		"before":                before,
		"lastObserved":          observed,
	}
	if result != nil {
		output["response"] = result
	}
	if writeErr != nil {
		output["writeError"] = writeErr.Error()
	}
	if observeErr != nil {
		output["verificationError"] = observeErr.Error()
	}
	_ = printJSON(output)
	return fmt.Errorf("clearing outcome for payment %s requires manual verification; no retry performed", input.PaymentDocumentNumber)
}

func clearingDocumentNumbers(input creditorClearingInput) []string {
	numbers := make([]string, 0, len(input.Items)+1)
	numbers = append(numbers, input.PaymentDocumentNumber)
	for _, item := range input.Items {
		numbers = append(numbers, item.DocumentNumber)
	}
	return numbers
}
