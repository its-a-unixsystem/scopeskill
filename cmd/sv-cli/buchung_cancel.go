package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungCancelUsage = `usage: sv-cli buchung cancel <documentNumber> [--row=N] [--date=YYYY-MM-DD] [--dry-run] [--yes]

Cancels the active Buchung <documentNumber>. Without --row, uses POST /journal/<documentNumber>/cancel.
With --row, uses POST /posting/cancel with the selected row and optional cancellation date.

Safety:
  --row      cancel one positive pde row number via /posting/cancel
  --date     cancellation date (YYYY-MM-DD); requires --row
  --dry-run   state discovery, preflight checks and preview only; no write
  --yes       bypass the interactive "cancel <documentNumber>" confirmation

A cancellation is only reported after the provider-linked Storno (an exact
sign-reversed Buchung whose rows carry cancellationNumber=<original
pdeRowNumber>) is read back from the journal.

Statuses on stdout: dry_run, cancelled, already_cancelled, conflict,
verification_required. Any status other than cancelled or already_cancelled
exits non-zero.`

const buchungReplaceUsage = `usage: sv-cli buchung replace <documentNumber> --data @replacement.json [--dry-run] [--yes]

Replaces the Buchung <documentNumber> with the reviewed replacement payload.

Currently unavailable: the atomic semantics of /postings/correction have not
passed the mandatory controlled live contract test. Use a separately
confirmed "buchung cancel" followed by "buchung create" instead.`

type cancellationState struct {
	stornoNumber      string
	stornoRecords     []any
	replacementNumber string
	linkedNumbers     []string
}

func buchungCancel(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung cancel", flag.ContinueOnError)
	rowValue := flags.String("row", "", "cancel one pde row number")
	dateValue := flags.String("date", "", "cancellation date (YYYY-MM-DD)")
	flags.SetOutput(cliError)
	dryRun := flags.Bool("dry-run", false, "state discovery, preflight checks and preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungCancelUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		flags.Usage()
		return errors.New("buchung cancel takes exactly one documentNumber")
	}
	documentNumber := flags.Arg(0)
	var rowNumber int
	rowMode := *rowValue != ""
	if rowMode {
		parsed, err := strconv.Atoi(*rowValue)
		if err != nil || parsed <= 0 {
			return errors.New("--row must be a positive integer")
		}
		rowNumber = parsed
	}
	if *dateValue != "" && !rowMode {
		return errors.New("--date requires --row")
	}
	cancellationDate := ""
	if *dateValue != "" {
		date, err := time.Parse(isoDateFormat, *dateValue)
		if err != nil || date.Format(isoDateFormat) != *dateValue {
			return errors.New("--date must be a date in YYYY-MM-DD format")
		}
		cancellationDate = date.Format("02.01.2006")
	}
	if strings.ContainsAny(documentNumber, "*?") {
		return fmt.Errorf("buchung cancel requires an exact documentNumber, got %q", documentNumber)
	}

	originalRecords, err := fetchCompleteJournal(client, documentNumber)
	if err != nil {
		return cancelVerificationRequired(documentNumber, "", "", nil, nil, err, nil)
	}
	if len(originalRecords) == 0 {
		return cancelConflict(documentNumber, fmt.Sprintf("documentNumber %s not found or has no journal rows", documentNumber), nil)
	}
	for _, item := range originalRecords {
		rec, ok := item.(map[string]any)
		if !ok {
			return cancelConflict(documentNumber, "journal returned a malformed record", nil)
		}
		if nonEmptyString(rec["cancellationNumber"]) != "" {
			return cancelConflict(documentNumber, fmt.Sprintf("documentNumber %s is itself a cancellation document and cannot be cancelled", documentNumber), nil)
		}
	}
	original, err := scopeskill.CanonicalFromJournal(originalRecords)
	if err != nil {
		return cancelConflict(documentNumber, fmt.Sprintf("journal records are inconsistent: %v", err), nil)
	}
	if original.DocumentNumber != documentNumber {
		return cancelConflict(documentNumber, fmt.Sprintf("journal returned documentNumber %q instead of %q", original.DocumentNumber, documentNumber), nil)
	}
	if original.PostingDate == "" {
		return cancelConflict(documentNumber, fmt.Sprintf("journal rows for %s carry no parseable postingDate", documentNumber), nil)
	}
	originalPDENumber, err := sharedJournalNumber(originalRecords, "pdeRowNumber")
	if err != nil {
		return cancelConflict(documentNumber, fmt.Sprintf("journal records carry no consistent pdeRowNumber: %v", err), nil)
	}
	state := cancellationState{}
	if rowMode {
		if strings.TrimSpace(originalPDENumber) != strconv.Itoa(rowNumber) {
			return cancelConflict(documentNumber, fmt.Sprintf("pdeRowNumber %d does not belong to documentNumber %s", rowNumber, documentNumber), nil)
		}
	} else {
		state, err = discoverCancellations(client, original, originalPDENumber)
		if err != nil {
			return cancelVerificationRequired(documentNumber, "", "", state.linkedNumbers, nil, err, nil)
		}
		if state.stornoNumber == "" && len(state.linkedNumbers) > 0 {
			return cancelConflict(documentNumber, "journal links exist but none is a verified sign-reversed Storno; refusing to write", state.linkedNumbers)
		}
	}
	if !rowMode && state.stornoNumber != "" {
		out := map[string]any{
			"status": "already_cancelled", "state": "already_cancelled",
			"originalDocumentNumber": documentNumber, "cancellationDocumentNumber": state.stornoNumber,
			"original":     map[string]any{"documentNumber": documentNumber, "rows": originalRecords},
			"cancellation": map[string]any{"documentNumber": state.stornoNumber, "rows": state.stornoRecords},
		}
		if state.replacementNumber != "" {
			out["replacementDocumentNumber"] = state.replacementNumber
		}
		return printJSON(out)
	}
	postingDate, err := time.Parse(isoDateFormat, original.PostingDate)
	if err != nil {
		return cancelConflict(documentNumber, fmt.Sprintf("cannot parse original postingDate %q", original.PostingDate), nil)
	}

	periodDate := postingDate
	if *dateValue != "" {
		periodDate, err = time.Parse(isoDateFormat, *dateValue)
		if err != nil {
			return cancelConflict(documentNumber, fmt.Sprintf("cannot parse cancellation date %q", *dateValue), nil)
		}
	}
	years, err := scopeskill.FetchFiscalYears(client)
	if err != nil {
		return cancelVerificationRequired(documentNumber, "active", "", nil, nil, err, nil)
	}
	fy, period, ok := scopeskill.FiscalPeriodFor(years, periodDate)
	if !ok {
		return cancelConflict(documentNumber, fmt.Sprintf("no fiscal period contains cancellation date %s", periodDate.Format(isoDateFormat)), nil)
	}
	if !fy.Open || !period.Open {
		return cancelConflict(documentNumber, fmt.Sprintf("fiscal period %q containing cancellation date %s is closed", period.Name, periodDate.Format(isoDateFormat)), nil)
	}

	accounts := map[string]*resolvedAccount{}
	seen := map[string]bool{}
	for _, row := range original.Rows {
		if seen[row.Account] {
			continue
		}
		seen[row.Account] = true
		account, err := resolvePostingAccount(client, accounts, row.Account)
		if err != nil {
			return cancelVerificationRequired(documentNumber, "active", "", nil, nil, err, nil)
		}
		if account == nil {
			return cancelConflict(documentNumber, fmt.Sprintf("account %s not found", row.Account), nil)
		}
		if !account.active() {
			return cancelConflict(documentNumber, fmt.Sprintf("account %s is inactive", row.Account), nil)
		}
	}

	raw, err := client.JSON(http.MethodGet, "/myaccount", nil, nil)
	if err != nil {
		return cancelVerificationRequired(documentNumber, "active", "", nil, nil, err, nil)
	}
	myaccount, _ := raw.(map[string]any)
	org, _ := myaccount["organisation"].(map[string]any)
	if org == nil {
		return cancelVerificationRequired(documentNumber, "active", "", nil, nil, errors.New("/myaccount response lacks the organisation object"), nil)
	}
	organisation := map[string]any{"id": org["id"], "name": org["name"]}
	path := "/journal/" + url.PathEscape(documentNumber) + "/cancel"
	var payload any
	if rowMode {
		path = "/posting/cancel"
		// Scopevisio uses pdeRowNumber only when documentNumber is absent.
		payload = map[string]any{"documentNumber": documentNumber, "pdeRowNumber": rowNumber}
		if cancellationDate != "" {
			payload.(map[string]any)["cancellationDate"] = cancellationDate
		}
	}
	req := writeRequest{
		Command: "buchung cancel", Method: http.MethodPost, Path: path, Payload: payload,
		Preview:       map[string]any{"state": "active", "requiredProfile": "Journal (Bearbeiten)", "organisation": organisation, "fiscalYear": fy.Name, "fiscalPeriod": period.Name, "original": map[string]any{"documentNumber": documentNumber, "rows": originalRecords}, "request": payload},
		ConfirmPhrase: "cancel " + documentNumber,
	}
	writePreview(client, req)

	if *dryRun {
		return printJSON(map[string]any{"status": "dry_run", "state": "active", "originalDocumentNumber": documentNumber, "endpoint": "POST " + req.Path, "organisation": organisation, "original": map[string]any{"documentNumber": documentNumber, "rows": originalRecords}, "request": payload})
	}

	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		_ = printJSON(map[string]any{
			"status":                 "conflict",
			"state":                  "active",
			"originalDocumentNumber": documentNumber,
			"reason":                 err.Error(),
		})
		return err
	}

	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome == writeRejected {
		_ = printJSON(map[string]any{
			"status":                 "conflict",
			"state":                  "conflict",
			"originalDocumentNumber": documentNumber,
			"reason":                 writeErr.Error(),
		})
		return writeErr
	}
	return verifyCancelledBuchung(client, original, originalPDENumber, result, outcome, writeErr)
}

func fetchCompleteJournal(client *scopeskill.Client, documentNumber string) ([]any, error) {
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

func sharedJournalNumber(records []any, field string) (string, error) {
	var shared string
	for i, item := range records {
		record, ok := item.(map[string]any)
		if !ok {
			return "", fmt.Errorf("journal record %d is not an object", i)
		}
		value := strings.TrimSpace(nonEmptyString(record[field]))
		if value == "" {
			return "", fmt.Errorf("journal record %d lacks %s", i, field)
		}
		if shared != "" && value != shared {
			return "", fmt.Errorf("journal record %d: %s %q conflicts with earlier value %q", i, field, value, shared)
		}
		shared = value
	}
	return shared, nil
}

func discoverCancellations(client *scopeskill.Client, original scopeskill.CanonicalBuchung, originalPDENumber string) (cancellationState, error) {
	var state cancellationState
	req := scopeskill.SearchRequest{
		PageSize: scopeskill.MaxSearchPageSize,
		Fields:   append(append([]string{}, journalSearchDefaultFields...), "pdeRowNumber", "cancellationNumber"),
		Conditions: []scopeskill.SearchCondition{
			{Field: "cancellationNumber", Operator: scopeskill.OpEquals, Value: originalPDENumber},
		},
	}
	body, err := req.Body()
	if err != nil {
		return state, err
	}
	raw, err := client.JSON(http.MethodPost, "/journal", body, nil)
	if err != nil {
		return state, err
	}
	rows, err := scopeskill.RecordsFromResponse(raw)
	if err != nil {
		return state, err
	}
	if len(rows) >= scopeskill.MaxSearchPageSize {
		return state, fmt.Errorf("journal linkage search returned a full page of %d records; result may be incomplete", len(rows))
	}
	for _, item := range rows {
		rec, ok := item.(map[string]any)
		if !ok {
			return state, errors.New("journal linkage search returned a malformed record")
		}
		if nonEmptyString(rec["cancellationNumber"]) != originalPDENumber {
			return state, fmt.Errorf("journal linkage search returned a row without cancellationNumber=%s", originalPDENumber)
		}
		number := nonEmptyString(rec["documentNumber"])
		if number == "" {
			return state, errors.New("journal linkage search row lacks documentNumber")
		}
		if !containsString(state.linkedNumbers, number) {
			state.linkedNumbers = append(state.linkedNumbers, number)
		}
	}
	for _, number := range state.linkedNumbers {
		records, err := fetchCompleteJournal(client, number)
		if err != nil {
			return state, err
		}
		if len(records) == 0 {
			return state, fmt.Errorf("linked document %s is missing or has no journal rows", number)
		}
		for _, item := range records {
			rec, ok := item.(map[string]any)
			if !ok {
				return state, fmt.Errorf("linked document %s returned a malformed record", number)
			}
			if nonEmptyString(rec["cancellationNumber"]) != originalPDENumber {
				return state, fmt.Errorf("linked document %s has a row without cancellationNumber=%s", number, originalPDENumber)
			}
		}
		canonical, err := scopeskill.CanonicalFromJournal(records)
		if err != nil {
			return state, fmt.Errorf("linked document %s is inconsistent: %w", number, err)
		}
		want, err := scopeskill.CanonicalCancellation(original, number)
		if err != nil {
			return state, err
		}
		want.DocumentText = ""
		if diff := scopeskill.CompareBuchung(want, canonical, false); diff.Equal() {
			if state.stornoNumber != "" {
				return state, fmt.Errorf("multiple Stornos found for %s", original.DocumentNumber)
			}
			state.stornoNumber = number
			state.stornoRecords = records
			continue
		}
		if state.replacementNumber != "" {
			return state, fmt.Errorf("multiple non-Storno documents linked to %s", original.DocumentNumber)
		}
		state.replacementNumber = number
	}
	return state, nil
}

func canonicalBuchungEqual(left, right scopeskill.CanonicalBuchung) bool {
	return scopeskill.CompareBuchung(left, right, false).Equal() &&
		scopeskill.CompareBuchung(right, left, false).Equal()
}

func cancelConflict(documentNumber, reason string, relatedDocumentNumbers []string) error {
	out := map[string]any{
		"status":                 "conflict",
		"state":                  "conflict",
		"originalDocumentNumber": documentNumber,
		"reason":                 reason,
	}
	if len(relatedDocumentNumbers) > 0 {
		out["relatedDocumentNumbers"] = relatedDocumentNumbers
	}
	_ = printJSON(out)
	return errors.New(reason)
}

func cancelVerificationRequired(documentNumber string, lastVerifiedState string, cancellationNumber string, relatedDocumentNumbers []string, result any, cause error, writeErr error) error {
	out := map[string]any{
		"status":                 "verification_required",
		"state":                  "verification_required",
		"originalDocumentNumber": documentNumber,
	}
	if lastVerifiedState != "" {
		out["lastVerifiedState"] = lastVerifiedState
	}
	if cancellationNumber != "" {
		out["cancellationDocumentNumber"] = cancellationNumber
	}
	if len(relatedDocumentNumbers) > 0 {
		out["relatedDocumentNumbers"] = relatedDocumentNumbers
	}
	if result != nil {
		out["response"] = result
	}
	if cause != nil {
		out["reason"] = cause.Error()
	}
	if writeErr != nil {
		out["writeError"] = writeErr.Error()
	}
	_ = printJSON(out)
	message := fmt.Sprintf("cancellation of documentNumber %s could not be verified; verify manually before retrying", documentNumber)
	if writeErr != nil {
		message += "; write error: " + writeErr.Error()
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func knownCancellationNumber(result any) string {
	switch v := result.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"cancellation documentNumber", "cancellationDocumentNumber", "documentNumber"} {
			if s := strings.TrimSpace(nonEmptyString(v[key])); s != "" {
				return s
			}
		}
	}
	return ""
}

func verifyCancelledBuchung(client *scopeskill.Client, original scopeskill.CanonicalBuchung, originalPDENumber string, result any, outcome writeOutcome, writeErr error) error {
	known := knownCancellationNumber(result)

	var originalErr error
	var readBackOriginal []any
	reread, err := fetchCompleteJournal(client, original.DocumentNumber)
	switch {
	case err != nil:
		originalErr = err
	case len(reread) == 0:
		originalErr = fmt.Errorf("original %s is no longer present in the journal", original.DocumentNumber)
	default:
		rereadPDENumber, err := sharedJournalNumber(reread, "pdeRowNumber")
		if err != nil {
			originalErr = fmt.Errorf("re-read original has no consistent pdeRowNumber: %w", err)
		} else if rereadPDENumber != originalPDENumber {
			originalErr = fmt.Errorf("original pdeRowNumber changed from %s to %s", originalPDENumber, rereadPDENumber)
		} else if rereadCanonical, err := scopeskill.CanonicalFromJournal(reread); err != nil {
			originalErr = fmt.Errorf("re-read original is inconsistent: %w", err)
		} else if !canonicalBuchungEqual(original, rereadCanonical) {
			originalErr = fmt.Errorf("original %s changed during cancellation", original.DocumentNumber)
		} else {
			readBackOriginal = reread
		}
	}

	state, discoverErr := discoverCancellations(client, original, originalPDENumber)

	if originalErr == nil && discoverErr == nil && state.stornoNumber != "" {
		out := map[string]any{
			"status":                     "cancelled",
			"state":                      "already_cancelled",
			"originalDocumentNumber":     original.DocumentNumber,
			"cancellationDocumentNumber": state.stornoNumber,
			"original":                   map[string]any{"documentNumber": original.DocumentNumber, "rows": readBackOriginal},
			"cancellation":               map[string]any{"documentNumber": state.stornoNumber, "rows": state.stornoRecords},
		}
		if state.replacementNumber != "" {
			out["replacementDocumentNumber"] = state.replacementNumber
		}
		if result != nil {
			out["response"] = result
		}
		if outcome == writeAmbiguous {
			out["writeResponse"] = "ambiguous"
			if writeErr != nil {
				out["writeError"] = writeErr.Error()
			}
		}
		return printJSON(out)
	}

	cause := originalErr
	if cause == nil {
		cause = discoverErr
	}
	if cause == nil && known != "" {
		cause = fmt.Errorf("provider-reported cancellation %s was not verified in the journal", known)
	}
	if cause == nil {
		cause = errors.New("no verified Storno found in the journal")
	}
	cancellationNumber := state.stornoNumber
	if cancellationNumber == "" {
		cancellationNumber = known
	}
	return cancelVerificationRequired(original.DocumentNumber, "active", cancellationNumber, state.linkedNumbers, result, cause, writeErr)
}

const buchungReplaceCapabilityProven = false

func buchungReplace(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung replace", flag.ContinueOnError)
	flags.SetOutput(cliError)
	data := flags.String("data", "", "replacement Buchung JSON, or @path/to/file.json")
	flags.Bool("dry-run", false, "preflight checks and preview only; no write")
	flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungReplaceUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" || *data == "" {
		flags.Usage()
		return errors.New("buchung replace takes exactly one documentNumber and requires --data")
	}
	documentNumber := flags.Arg(0)
	if strings.ContainsAny(documentNumber, "*?") {
		return fmt.Errorf("buchung replace requires an exact documentNumber, got %q", documentNumber)
	}
	if !buchungReplaceCapabilityProven {
		reason := "buchung replace is unavailable: /postings/correction atomic cancellation semantics have not been proven by the required controlled live contract test; use a separately confirmed 'buchung cancel' followed by 'buchung create'"
		_ = printJSON(map[string]any{
			"status":                 "conflict",
			"state":                  "conflict",
			"originalDocumentNumber": documentNumber,
			"reason":                 reason,
		})
		return errors.New(reason)
	}
	return errors.New("buchung replace capability is marked proven but is not implemented; refusing to write")
}
