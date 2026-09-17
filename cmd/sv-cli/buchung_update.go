package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungUpdateUsage = `usage: sv-cli buchung update <documentNumber> [flags]

Updates internal and/or external document numbers for an existing posting via PUT /postings/update.
Under GoBD rules, accounts, amounts, tax keys, and dates are immutable on existing postings; only document numbers may be adjusted.

Flags:
  --internal-number=STR    set internalDocumentNumber
  --external-number=STR    set externalDocumentNumber
  --file=changes.json      JSON file containing internalDocumentNumber and/or externalDocumentNumber
  --dry-run                preview only; no write
  --yes                    skip the interactive confirmation

At least one modification flag or file field is required.`

func buchungUpdate(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung update", flag.ContinueOnError)
	flags.SetOutput(cliError)
	internalNumber := flags.String("internal-number", "", "internalDocumentNumber")
	externalNumber := flags.String("external-number", "", "externalDocumentNumber")
	file := flags.String("file", "", "path to JSON file")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungUpdateUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		flags.Usage()
		return errors.New("buchung update requires exactly one documentNumber")
	}

	docNum := flags.Arg(0)
	payload := map[string]any{
		"documentNumber": docNum,
	}

	if *file != "" {
		filePath := *file
		if !strings.HasPrefix(filePath, "@") {
			filePath = "@" + filePath
		}
		body, err := loadJSONObject(filePath)
		if err != nil {
			return err
		}
		if rawDocumentNumber, exists := body["documentNumber"]; exists {
			fileDocumentNumber, ok := rawDocumentNumber.(string)
			if !ok {
				return errors.New("documentNumber in --file must be a string")
			}
			if fileDocumentNumber != "" && fileDocumentNumber != docNum {
				return fmt.Errorf("documentNumber %q in file does not match %q", fileDocumentNumber, docNum)
			}
		}
		for _, field := range []string{"internalDocumentNumber", "externalDocumentNumber"} {
			rawValue, exists := body[field]
			if !exists {
				continue
			}
			value, ok := rawValue.(string)
			if !ok {
				return fmt.Errorf("%s in --file must be a string", field)
			}
			if value != "" {
				payload[field] = value
			}
		}
	}

	if *internalNumber != "" {
		payload["internalDocumentNumber"] = *internalNumber
	}
	if *externalNumber != "" {
		payload["externalDocumentNumber"] = *externalNumber
	}

	if _, hasInternal := payload["internalDocumentNumber"]; !hasInternal {
		if _, hasExternal := payload["externalDocumentNumber"]; !hasExternal {
			flags.Usage()
			return errors.New("buchung update requires at least one of --internal-number, --external-number, or a valid --file")
		}
	}

	req := writeRequest{
		Command:       "buchung update",
		Method:        http.MethodPut,
		Path:          "/postings/update",
		Payload:       payload,
		ConfirmPhrase: "update buchung " + docNum,
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":         "dry_run",
			"documentNumber": docNum,
			"endpoint":       "PUT /postings/update",
			"request":        payload,
		})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	_, outcome, err := executeWriteOnce(client, req, func(result any) bool { return true })
	if outcome == writeRejected || err != nil {
		return err
	}

	fetchNum := docNum
	if in, ok := payload["internalDocumentNumber"].(string); ok && in != "" {
		fetchNum = in
	}
	readback, err := client.JSON(http.MethodGet, "/journal/"+url.PathEscape(fetchNum), nil, nil)
	if err != nil {
		return fmt.Errorf("read back updated buchung: %w", err)
	}
	if readback == nil {
		return errors.New("read back updated buchung: empty journal response")
	}
	out := map[string]any{
		"status":         "updated",
		"documentNumber": docNum,
	}
	if in, ok := payload["internalDocumentNumber"].(string); ok && in != "" {
		out["internalDocumentNumber"] = in
	}
	if en, ok := payload["externalDocumentNumber"].(string); ok && en != "" {
		out["externalDocumentNumber"] = en
	}
	out["buchung"] = readback
	return printJSON(out)
}
