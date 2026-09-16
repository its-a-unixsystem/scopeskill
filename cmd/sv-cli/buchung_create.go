package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungCreateUsage = `usage: sv-cli buchung create --data @buchung.json [--dry-run] [--yes]

Input schema (--data JSON or @file):
  documentNumber            required; caller-supplied unique Belegnummer
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
already_exists exits non-zero.`

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

	existingRecords, err := fetchJournalRecords(client, in.DocumentNumber)
	if err != nil {
		return err
	}
	expected := in.Canonical()
	allowGenerated := in.AutoCreateTax != nil && *in.AutoCreateTax
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
		return printJSON(map[string]any{
			"status":         "already_exists",
			"documentNumber": in.DocumentNumber,
			"journal":        map[string]any{"rows": actual.Rows},
			"generated":      diff.Generated,
		})
	}
	if exists {
		_ = printJSON(map[string]any{
			"status":         "conflict",
			"documentNumber": in.DocumentNumber,
			"expected":       expected,
			"actual":         actual,
			"diff":           diff,
		})
		return fmt.Errorf("documentNumber %s already exists with different rows; no write performed", in.DocumentNumber)
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

	result, outcome, writeErr := executeWriteOnce(client, req, acceptPostingsNewResponse)
	switch outcome {
	case writeRejected:
		return writeErr
	case writeAccepted:
		return verifyCreatedBuchung(client, expected, payload, result, allowGenerated)
	default:
		return recoverAmbiguousWrite(client, expected, payload, result, allowGenerated, writeErr)
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
	raw, err := client.JSON(http.MethodGet, "/journal/"+url.PathEscape(documentNumber), nil, map[string]string{
		"fields": strings.Join(journalSearchDefaultFields, ","),
	})
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

func createdOutput(expected scopeskill.CanonicalBuchung, payload scopeskill.PostingsRequest, result any, records []any, diff scopeskill.BuchungDiff) map[string]any {
	return map[string]any{
		"status":         "created",
		"documentNumber": expected.DocumentNumber,
		"endpoint":       "POST /postings/new",
		"request":        payload,
		"response":       result,
		"journal":        map[string]any{"rows": records},
		"generated":      diff.Generated,
	}
}

func verifyCreatedBuchung(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, payload scopeskill.PostingsRequest, result any, allowGenerated bool) error {
	records, err := fetchJournalRecords(client, expected.DocumentNumber)
	if err != nil {
		return err
	}
	actual, err := scopeskill.CanonicalFromJournal(records)
	if err != nil {
		return err
	}
	diff := scopeskill.CompareBuchung(expected, actual, allowGenerated)
	if !diff.Equal() {
		_ = printJSON(map[string]any{
			"status":         "verification_failed",
			"documentNumber": expected.DocumentNumber,
			"response":       result,
			"expected":       expected,
			"actual":         actual,
			"diff":           diff,
		})
		return errors.New("post-write journal does not match the requested Buchung")
	}
	return printJSON(createdOutput(expected, payload, result, records, diff))
}

func recoverAmbiguousWrite(client *scopeskill.Client, expected scopeskill.CanonicalBuchung, payload scopeskill.PostingsRequest, result any, allowGenerated bool, writeErr error) error {
	writeError := ""
	if writeErr != nil {
		writeError = writeErr.Error()
	}
	records, err := fetchJournalRecords(client, expected.DocumentNumber)
	if err == nil && len(records) > 0 {
		if actual, err := scopeskill.CanonicalFromJournal(records); err == nil {
			if diff := scopeskill.CompareBuchung(expected, actual, allowGenerated); diff.Equal() {
				out := createdOutput(expected, payload, result, records, diff)
				out["writeResponse"] = "ambiguous"
				out["writeError"] = writeError
				return printJSON(out)
			}
		}
	}
	_ = printJSON(map[string]any{
		"status":         "verification_required",
		"documentNumber": expected.DocumentNumber,
		"writeError":     writeError,
		"response":       result,
	})
	return fmt.Errorf("write response ambiguous for documentNumber %s; verify manually before retrying", expected.DocumentNumber)
}
