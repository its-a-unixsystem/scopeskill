package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const eingangsrechnungUpdateUsage = `usage: sv-cli eingangsrechnung update <idOrNumber> [flags] [--dry-run] [--yes]

Repairs an unverified Eingangsrechnung via POST /incominginvoice/{id}.

Only the flags you pass are sent; every other Beleg field is left untouched.
Dates are accepted as YYYY-MM-DD and sent as epoch milliseconds.

Flags:
  --vendor-contact-id=N   Lieferant/Kreditor Kontakt Master-ID
  --document-number=VALUE Rechnungsnummer (external document number)
  --document-date=DATE    Rechnungsdatum (YYYY-MM-DD)
  --due-date=DATE         Fälligkeitsdatum (YYYY-MM-DD)
  --delivery-date-from=DATE Leistungsdatum (YYYY-MM-DD)
  --delivery-date-to=DATE     Leistungsdatum bis (YYYY-MM-DD)
  --text=VALUE            Belegtext (text1)

Safety:
  --dry-run   fetch, no-change check and payload preview only; no write
  --yes       bypass the interactive "update <idOrNumber>" confirmation

At least one flag must be set. Update is only possible while the Beleg is not
verified. Stdout statuses: dry_run, updated, already_up_to_date,
verification_required. Any status other than updated or already_up_to_date
exits non-zero.`

// belegUpdateLocation is the location Scopevisio date fields are interpreted
// in when the provider converts epoch milliseconds back to calendar days.
const belegUpdateLocation = "Europe/Berlin"

type belegUpdateRequest struct {
	vendorContactID  string
	documentNumber   string
	documentDate     string
	dueDate          string
	deliveryDateFrom string
	deliveryDateTo   string
	text             string
}

func (r belegUpdateRequest) empty() bool {
	return r.vendorContactID == "" && r.documentNumber == "" && r.documentDate == "" &&
		r.dueDate == "" && r.deliveryDateFrom == "" && r.deliveryDateTo == "" && r.text == ""
}

// payload converts the requested flags into the partial IncomingInvoiceUpdateForm:
// only requested fields are present, ISO dates become epoch milliseconds at UTC
// midnight.
func (r belegUpdateRequest) payload() (map[string]any, error) {
	body := map[string]any{}
	if r.vendorContactID != "" {
		id, err := strconv.ParseInt(r.vendorContactID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--vendor-contact-id must be an integer: %w", err)
		}
		body["vendorContactId"] = id
	}
	if r.documentNumber != "" {
		body["documentNumber"] = r.documentNumber
	}
	dates := []struct {
		flag  string
		value string
		field string
	}{
		{"--document-date", r.documentDate, "documentDate"},
		{"--due-date", r.dueDate, "dueDate"},
		{"--delivery-date-from", r.deliveryDateFrom, "deliveryDateFrom"},
		{"--delivery-date-to", r.deliveryDateTo, "deliveryDateTo"},
	}
	for _, d := range dates {
		if d.value == "" {
			continue
		}
		millis, err := isoDateToMillis(d.flag, d.value)
		if err != nil {
			return nil, err
		}
		body[d.field] = millis
	}
	if r.text != "" {
		body["text1"] = r.text
	}
	return body, nil
}

func isoDateToMillis(flagName, value string) (int64, error) {
	day, err := time.Parse(isoDateFormat, value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a date in YYYY-MM-DD format: %w", flagName, err)
	}
	return day.Unix() * 1000, nil
}

func eingangsrechnungUpdate(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("eingangsrechnung update", flag.ContinueOnError)
	flags.SetOutput(cliError)
	var requested belegUpdateRequest
	flags.StringVar(&requested.vendorContactID, "vendor-contact-id", "", "Lieferant/Kreditor Kontakt Master-ID")
	flags.StringVar(&requested.documentNumber, "document-number", "", "Rechnungsnummer")
	flags.StringVar(&requested.documentDate, "document-date", "", "Rechnungsdatum (YYYY-MM-DD)")
	flags.StringVar(&requested.dueDate, "due-date", "", "Fälligkeitsdatum (YYYY-MM-DD)")
	flags.StringVar(&requested.deliveryDateFrom, "delivery-date-from", "", "Leistungsdatum (YYYY-MM-DD)")
	flags.StringVar(&requested.deliveryDateTo, "delivery-date-to", "", "Leistungsdatum bis (YYYY-MM-DD)")
	flags.StringVar(&requested.text, "text", "", "Belegtext (text1)")
	dryRun := flags.Bool("dry-run", false, "fetch, no-change check and payload preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, eingangsrechnungUpdateUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		flags.Usage()
		return errors.New("eingangsrechnung update takes exactly one idOrNumber")
	}
	idOrNumber := flags.Arg(0)
	if strings.ContainsAny(idOrNumber, "*?") {
		return fmt.Errorf("eingangsrechnung update requires an exact id or number, got %q", idOrNumber)
	}
	if requested.empty() {
		flags.Usage()
		return errors.New("eingangsrechnung update requires at least one field flag")
	}

	body, err := requested.payload()
	if err != nil {
		return err
	}

	beleg, err := scopeskill.FetchBeleg(client, scopeskill.BelegEndpointIncomingInvoice, idOrNumber)
	if err != nil {
		return err
	}
	if beleg == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}
	if changes := pendingBelegChanges(beleg, body); len(changes) == 0 {
		return printJSON(map[string]any{
			"status":           "already_up_to_date",
			"idOrNumber":       idOrNumber,
			"eingangsrechnung": beleg,
		})
	}
	req := writeRequest{
		Command:       "eingangsrechnung update",
		Method:        http.MethodPost,
		Path:          fmt.Sprintf("%s/%s", scopeskill.BelegEndpointIncomingInvoice, url.PathEscape(idOrNumber)),
		Payload:       body,
		ConfirmPhrase: "update " + idOrNumber,
	}
	writePreview(client, req)

	output := map[string]any{
		"endpoint": "POST " + req.Path,
		"request":  body,
	}
	if *dryRun {
		output["status"] = "dry_run"
		return printJSON(output)
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome == writeRejected {
		return writeErr
	}

	reread, err := scopeskill.FetchBeleg(client, scopeskill.BelegEndpointIncomingInvoice, idOrNumber)
	if outcome == writeAmbiguous {
		return belegUpdateVerificationRequired(idOrNumber, result, err, writeErr)
	}
	if err != nil {
		return belegUpdateVerificationRequired(idOrNumber, result, err, nil)
	}
	if reread == nil {
		return belegUpdateVerificationRequired(idOrNumber, result, errors.New("Eingangsrechnung could not be read back after the update"), nil)
	}
	if missing := pendingBelegChanges(reread, body); len(missing) > 0 {
		return belegUpdateVerificationRequired(idOrNumber, result, fmt.Errorf("fields not applied: %s", strings.Join(missing, ", ")), nil)
	}

	output["status"] = "updated"
	output["response"] = result
	output["eingangsrechnung"] = reread
	return printJSON(output)
}

// belegUpdateDateFields are the IncomingInvoiceUpdateForm fields the provider
// stores as epoch-millisecond timestamps.
var belegUpdateDateFields = map[string]bool{
	"documentDate":     true,
	"dueDate":          true,
	"deliveryDateFrom": true,
	"deliveryDateTo":   true,
}

// pendingBelegChanges returns the requested fields whose current value on the
// Beleg does not match the requested update value. Date fields are compared as
// calendar days in the provider's location, so an epoch written at UTC midnight
// round-trips against a provider timestamp anchored in Europe/Berlin; every
// other field is compared exactly.
func pendingBelegChanges(beleg map[string]any, body map[string]any) []string {
	location, err := time.LoadLocation(belegUpdateLocation)
	if err != nil {
		location = time.UTC
	}
	var missing []string
	for field, requested := range body {
		if belegUpdateValuesEqual(beleg[field], requested, belegUpdateDateFields[field], location) {
			continue
		}
		missing = append(missing, field)
	}
	slices.Sort(missing)
	return missing
}

func belegUpdateValuesEqual(current any, requested any, dayGranularity bool, location *time.Location) bool {
	if dayGranularity {
		currentMillis, currentOK := intValue(current)
		requestedMillis, requestedOK := intValue(requested)
		if currentOK && requestedOK {
			return time.UnixMilli(currentMillis).In(location).Format(isoDateFormat) ==
				time.UnixMilli(requestedMillis).In(location).Format(isoDateFormat)
		}
	}
	switch current := current.(type) {
	case string:
		requested, ok := requested.(string)
		return ok && current == requested
	case int64:
		requested, ok := intValue(requested)
		return ok && current == requested
	case float64:
		requested, ok := intValue(requested)
		return ok && int64(current) == requested
	}
	return false
}

// intValue extracts an integer from the JSON-decoded numeric types the API
// returns (float64) and the CLI sends (int64).
func intValue(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
}

func belegUpdateVerificationRequired(idOrNumber string, result any, cause error, writeErr error) error {
	out := map[string]any{
		"status":     "verification_required",
		"idOrNumber": idOrNumber,
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
	message := fmt.Sprintf("update of Eingangsrechnung %s could not be verified; verify manually before retrying", idOrNumber)
	if writeErr != nil {
		message += "; write error: " + writeErr.Error()
	}
	if cause != nil {
		message += ": " + cause.Error()
	}
	return errors.New(message)
}
