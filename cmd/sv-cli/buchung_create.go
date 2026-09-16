package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungCreateUsage = `usage: sv-cli buchung create --data @buchung.json [--dry-run] [--yes]

Input schema (--data JSON or @file):
  documentNumber            required request identifier (Scopevisio can assign a different Belegnummer)
  postingDate               required; YYYY-MM-DD
  documentDate              optional; YYYY-MM-DD
  externalDocumentNumber    optional
  internalDocumentNumber    optional
  documentText              optional
  autoCreateTax             optional bool; passed through verbatim
  adjustVatKey              optional bool; passed through verbatim
  rows                      required; at least two PostingRows

Row fields:
  account                   required; Sachkonto, Debitor, or Kreditor number
  summaryAccount            required when account is a Personenkonto
  amount                    required; EUR with at most two decimals, non-zero
  rowText, vatKey           optional
  dimensions                optional [{dimensionId: "dimension_N", dimensionAccountNumber}]
  foreignCurrencyAmount/Code/Rate, discountPercent1/2, discountPeriod1/2,
  netTimeLimit, discountAccount, paymentType, ustId   optional

Safety:
  --dry-run   preflight checks + payload preview only; no write request
  --yes       bypass the interactive "create <documentNumber>" confirmation

Statuses on stdout: dry_run, created, already_exists, conflict,
verification_required, verification_failed. Any status other than created or
already_exists exits non-zero. documentNumber contains the final Scopevisio
number. requestedDocumentNumber contains the input number when they differ.`

const providerPostingDiscoveryLookback = time.Minute

var errProviderDocumentNotFound = errors.New("provider-assigned documentNumber not found")

type resolvedAccount struct {
	record   map[string]any
	personal bool
}

func (a resolvedAccount) active() bool {
	active, _ := a.record["active"].(bool)
	return active
}

func buchungCreate(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung create", flag.ContinueOnError)
	flags.SetOutput(cliError)
	data := flags.String("data", "", "Buchung JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preflight checks and payload preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungCreateUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *data == "" {
		flags.Usage()
		return errors.New("buchung create requires --data and takes no positional arguments")
	}

	raw, err := loadRaw(*data)
	if err != nil {
		return err
	}
	in, err := scopeskill.ParseSinglePostingInput(raw)
	if err != nil {
		return err
	}

	postingDate, err := time.Parse(isoDateFormat, in.PostingDate)
	if err != nil {
		return err
	}
	years, err := scopeskill.FetchFiscalYears(client)
	if err != nil {
		return err
	}
	fy, period, ok := scopeskill.FiscalPeriodFor(years, postingDate)
	if !ok {
		return fmt.Errorf("no fiscal period contains postingDate %s", in.PostingDate)
	}
	if !fy.Open || !period.Open {
		return fmt.Errorf("fiscal period %q containing postingDate %s is closed", period.Name, in.PostingDate)
	}

	accounts := map[string]*resolvedAccount{}
	for i, row := range in.Rows {
		account, err := resolvePostingAccount(client, accounts, row.Account)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("rows[%d]: account %s not found", i+1, row.Account)
		}
		if !account.active() {
			return fmt.Errorf("rows[%d]: account %s is inactive", i+1, row.Account)
		}
		if !account.personal {
			continue
		}
		if row.SummaryAccount == "" {
			return fmt.Errorf("rows[%d]: summaryAccount is required for Personenkonto %s", i+1, row.Account)
		}
		summary, err := resolvePostingAccount(client, accounts, row.SummaryAccount)
		if err != nil {
			return err
		}
		if summary == nil || summary.personal {
			return fmt.Errorf("rows[%d]: summaryAccount %s not found", i+1, row.SummaryAccount)
		}
		if !summary.active() {
			return fmt.Errorf("rows[%d]: summaryAccount %s is inactive", i+1, row.SummaryAccount)
		}
	}

	if err := preflightVatKeys(client, in); err != nil {
		return err
	}

	expected := in.Canonical()
	allowGenerated := in.AutoCreateTax != nil && *in.AutoCreateTax
	reconciledExpected, existingRecords, err := fetchReconciledBuchung(client, expected, allowGenerated, time.Time{})
	if err != nil && !errors.Is(err, errProviderDocumentNotFound) {
		return err
	}
	expected = reconciledExpected
	var actual scopeskill.CanonicalBuchung
	var diff scopeskill.BuchungDiff
	exists := len(existingRecords) > 0
	if exists {
		actual, err = scopeskill.CanonicalFromJournal(existingRecords)
		if err != nil {
			return err
		}
		diff = scopeskill.CompareBuchung(expected, actual, allowGenerated)
	}

	payload, err := in.ToPostingsRequest()
	if err != nil {
		return err
	}
	req := writeRequest{
		Command:       "buchung create",
		Method:        http.MethodPost,
		Path:          "/postings/new",
		Payload:       payload,
		ConfirmPhrase: "create " + in.DocumentNumber,
	}
	writePreview(client, req)

	if exists && diff.Equal() {
		out := buchungStatusOutput("already_exists", expected.DocumentNumber, in.DocumentNumber)
		out["journal"] = map[string]any{"rows": actual.Rows}
		out["generated"] = diff.Generated
		return printJSON(out)
	}
	if exists {
		out := buchungStatusOutput("conflict", expected.DocumentNumber, in.DocumentNumber)
		out["expected"] = expected
		out["actual"] = actual
		out["diff"] = diff
		_ = printJSON(out)
		return fmt.Errorf("documentNumber %s already exists with different rows; no write performed", expected.DocumentNumber)
	}

	if *dryRun {
		return printJSON(map[string]any{
			"status":         "dry_run",
			"documentNumber": in.DocumentNumber,
			"endpoint":       "POST /postings/new",
			"request":        payload,
		})
	}

	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	createdSince := time.Now().Add(-providerPostingDiscoveryLookback)
	result, outcome, writeErr := executeWriteOnce(client, req, acceptPostingsNewResponse)
	switch outcome {
	case writeRejected:
		return writeErr
	case writeAccepted:
		return verifyCreatedBuchung(client, expected, payload, result, allowGenerated, createdSince)
	default:
		return recoverAmbiguousWrite(client, expected, payload, result, allowGenerated, writeErr, createdSince)
	}
}

func resolvePostingAccount(client *scopeskill.Client, cache map[string]*resolvedAccount, number string) (*resolvedAccount, error) {
	if cached, ok := cache[number]; ok {
		return cached, nil
	}
	var account *resolvedAccount
	rec, err := fetchSachkontoByNumber(client, number)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		account = &resolvedAccount{record: rec}
	} else {
		for _, kind := range []personalAccountKind{debitorAccountKind, kreditorAccountKind} {
			rec, err := fetchPersonalAccountByNumber(client, kind, number)
			if err != nil {
				return nil, err
			}
			if rec != nil {
				account = &resolvedAccount{record: rec, personal: true}
				break
			}
		}
	}
	cache[number] = account
	return account, nil
}

func preflightVatKeys(client *scopeskill.Client, in scopeskill.SinglePostingInput) error {
	needsVat := false
	for _, row := range in.Rows {
		if row.VatKey != "" {
			needsVat = true
			break
		}
	}
	if !needsVat {
		return nil
	}
	raw, err := client.JSON(http.MethodGet, "/vatmatrixentries", nil, nil)
	if err != nil {
		return err
	}
	records, err := scopeskill.RecordsFromResponse(raw)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, item := range records {
		rec, _ := item.(map[string]any)
		if key, ok := rec["vatKey"].(string); ok {
			known[key] = true
		}
	}
	for i, row := range in.Rows {
		if row.VatKey != "" && !known[row.VatKey] {
			return fmt.Errorf("rows[%d]: vatKey %s not found in Steuermatrix", i+1, row.VatKey)
		}
	}
	return nil
}

// fetchJournalRecords returns nil (no error) when the document number does not
// exist in the journal.
func fetchJournalRecords(client *scopeskill.Client, documentNumber string) ([]any, error) {
	// Ohne fields-Projektion abrufen: sie unterschlägt personalAccountNumber,
	// das der Kreditorenausgleich zum Erkennen des Personenkontos braucht
	// (scopeskill #58). Die vollständige Zeile ist klein genug.
	raw, err := client.JSON(http.MethodGet, "/journal/"+url.PathEscape(documentNumber), nil, nil)
	if err != nil {
		var apiErr scopeskill.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	return scopeskill.RecordsFromResponse(raw)
}

func acceptPostingsNewResponse(result any) bool {
	obj, ok := result.(map[string]any)
	if !ok {
		return false
	}
	if fmt.Sprint(obj["insertCount"]) != "1" {
		return false
	}
	update, present := obj["updateCount"]
	return !present || fmt.Sprint(update) == "0"
}

func buchungStatusOutput(status, documentNumber, requestedDocumentNumber string) map[string]any {
	out := map[string]any{
		"status":         status,
		"documentNumber": documentNumber,
	}
	if requestedDocumentNumber != documentNumber {
		out["requestedDocumentNumber"] = requestedDocumentNumber
	}
	return out
}

func createdOutput(expected scopeskill.CanonicalBuchung, requestedDocumentNumber string, payload scopeskill.PostingsRequest, result any, records []any, diff scopeskill.BuchungDiff) map[string]any {
	out := buchungStatusOutput("created", expected.DocumentNumber, requestedDocumentNumber)
	out["endpoint"] = "POST /postings/new"
	out["request"] = payload
	out["response"] = result
	out["journal"] = map[string]any{"rows": records}
	out["generated"] = diff.Generated
	return out
}

func discoverCreatedDocumentNumber(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, allowGenerated bool, createdSince time.Time) (string, error) {
	postingDate, err := time.Parse(isoDateFormat, expected.PostingDate)
	if err != nil {
		return "", err
	}
	request := scopeskill.SearchRequest{
		Fields: append([]string{}, journalSearchDefaultFields...),
	}
	records, err := scopeskill.Paginate(scopeskill.PaginateOptions{All: true}, request, func(body map[string]any) ([]any, error) {
		if !createdSince.IsZero() {
			body["createdSince"] = createdSince.UnixMilli()
		}
		body["postingDateSince"] = postingDate.UnixMilli()
		body["postingDateBefore"] = postingDate.AddDate(0, 0, 1).UnixMilli()
		raw, err := client.JSON(http.MethodPost, "/journal", body, nil)
		if err != nil {
			return nil, err
		}
		return scopeskill.RecordsFromResponse(raw)
	})
	if err != nil {
		return "", err
	}
	if len(records) >= scopeskill.DefaultMaxResults {
		return "", fmt.Errorf("journal search reached the %d-record safety cap while reconciling documentNumber %s", scopeskill.DefaultMaxResults, expected.DocumentNumber)
	}
	rowsByDocumentNumber := map[string][]any{}
	for _, item := range records {
		record, _ := item.(map[string]any)
		documentNumber := nonEmptyString(record["documentNumber"])
		if documentNumber != "" {
			rowsByDocumentNumber[documentNumber] = append(rowsByDocumentNumber[documentNumber], item)
		}
	}
	var matches []string
	for documentNumber, rows := range rowsByDocumentNumber {
		actual, err := scopeskill.CanonicalFromJournal(rows)
		if err != nil {
			continue
		}
		candidate := expected
		candidate.DocumentNumber = documentNumber
		if scopeskill.CompareBuchung(candidate, actual, allowGenerated).Equal() {
			matches = append(matches, documentNumber)
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%w for requested documentNumber %s", errProviderDocumentNotFound, expected.DocumentNumber)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple provider-assigned documentNumbers match requested documentNumber %s: %s", expected.DocumentNumber, strings.Join(matches, ", "))
	}
}

func fetchReconciledBuchung(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, allowGenerated bool, createdSince time.Time) (scopeskill.CanonicalBuchung, []any, error) {
	records, err := fetchJournalRecords(client, expected.DocumentNumber)
	if err != nil || len(records) > 0 {
		return expected, records, err
	}
	assignedDocumentNumber, err := discoverCreatedDocumentNumber(client, expected, allowGenerated, createdSince)
	if err != nil {
		return expected, nil, err
	}
	expected.DocumentNumber = assignedDocumentNumber
	records, err = fetchJournalRecords(client, assignedDocumentNumber)
	if err != nil {
		return expected, nil, err
	}
	if len(records) == 0 {
		return expected, nil, fmt.Errorf("provider-assigned documentNumber %s was found but could not be read back", assignedDocumentNumber)
	}
	return expected, records, nil
}

func verifyCreatedBuchung(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, payload scopeskill.PostingsRequest, result any, allowGenerated bool, createdSince time.Time) error {
	requestedDocumentNumber := expected.DocumentNumber
	expected, records, err := fetchReconciledBuchung(client, expected, allowGenerated, createdSince)
	if err != nil {
		out := buchungStatusOutput("verification_required", expected.DocumentNumber, requestedDocumentNumber)
		out["response"] = result
		_ = printJSON(out)
		return err
	}
	actual, err := scopeskill.CanonicalFromJournal(records)
	if err != nil {
		return err
	}
	diff := scopeskill.CompareBuchung(expected, actual, allowGenerated)
	if !diff.Equal() {
		out := buchungStatusOutput("verification_failed", expected.DocumentNumber, requestedDocumentNumber)
		out["response"] = result
		out["expected"] = expected
		out["actual"] = actual
		out["diff"] = diff
		_ = printJSON(out)
		return errors.New("post-write journal does not match the requested Buchung")
	}
	return printJSON(createdOutput(expected, requestedDocumentNumber, payload, result, records, diff))
}

func recoverAmbiguousWrite(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, payload scopeskill.PostingsRequest, result any, allowGenerated bool, writeErr error, createdSince time.Time) error {
	requestedDocumentNumber := expected.DocumentNumber
	writeError := ""
	if writeErr != nil {
		writeError = writeErr.Error()
	}
	expected, records, err := fetchReconciledBuchung(client, expected, allowGenerated, createdSince)
	if err == nil && len(records) > 0 {
		if actual, err := scopeskill.CanonicalFromJournal(records); err == nil {
			if diff := scopeskill.CompareBuchung(expected, actual, allowGenerated); diff.Equal() {
				out := createdOutput(expected, requestedDocumentNumber, payload, result, records, diff)
				out["writeResponse"] = "ambiguous"
				out["writeError"] = writeError
				return printJSON(out)
			}
		}
	}
	out := buchungStatusOutput("verification_required", expected.DocumentNumber, requestedDocumentNumber)
	out["writeError"] = writeError
	out["response"] = result
	_ = printJSON(out)
	return fmt.Errorf("write response ambiguous for documentNumber %s; verify manually before retrying", expected.DocumentNumber)
}
