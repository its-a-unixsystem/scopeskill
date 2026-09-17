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
		if fileDoc, ok := body["documentNumber"].(string); ok && fileDoc != "" && fileDoc != docNum {
			return fmt.Errorf("documentNumber %q in file does not match %q", fileDoc, docNum)
		}
		if v, ok := body["internalDocumentNumber"].(string); ok && v != "" {
			payload["internalDocumentNumber"] = v
		}
		if v, ok := body["externalDocumentNumber"].(string); ok && v != "" {
			payload["externalDocumentNumber"] = v
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

	// Read back the updated posting
	fetchNum := docNum
	if in, ok := payload["internalDocumentNumber"].(string); ok && in != "" {
		fetchNum = in
	}
	readback, readErr := client.JSON(http.MethodGet, "/journal/"+url.PathEscape(fetchNum), nil, nil)
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
	if readErr == nil && readback != nil {
		out["buchung"] = readback
	}
	return printJSON(out)
}
