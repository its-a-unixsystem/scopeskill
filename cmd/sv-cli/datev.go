package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const datevExportUsage = `usage: sv-cli datev export --from=YYYY-MM-DD --to=YYYY-MM-DD --out=export.csv [--fiscal-year=N]

Exports Buchungen as unmodified DATEV EXTF bytes from POST /datevexport.
The date range is sent as inclusive posting-date timestamps.`

const datevImportUsage = `usage: sv-cli datev import --file=datev.csv [--dry-run] [--yes]

Imports unmodified DATEV EXTF file bytes via POST /datevpostings/new. The file is
base64-encoded only because the Scopevisio API requires a DatevPostings JSON
envelope; sv-cli does not parse or validate its contents.

Safety:
  --dry-run   read the local file and preview metadata only; no API request
  --yes       bypass the interactive "import datev <filename>" confirmation`

type datevImportPayload struct {
	Data string `json:"data"`
}

func datev(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "datev subcommands: export import")
		return errors.New("missing datev subcommand")
	}
	switch args[0] {
	case "export":
		return datevExport(client, args[1:])
	case "import":
		return datevImport(client, args[1:])
	default:
		return fmt.Errorf("unknown datev command: %s", args[0])
	}
}

func datevExport(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("datev export", flag.ContinueOnError)
	flags.SetOutput(cliError)
	from := flags.String("from", "", "first posting date (YYYY-MM-DD)")
	to := flags.String("to", "", "last posting date (YYYY-MM-DD), inclusive")
	out := flags.String("out", "", "output file path")
	fiscalYear := flags.Int("fiscal-year", 0, "fiscal year number")
	flags.Usage = func() { fmt.Fprintln(cliError, datevExportUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *from == "" || *to == "" || *out == "" {
		flags.Usage()
		return errors.New("datev export requires --from, --to, and --out")
	}

	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return err
	}
	fromDate, err := time.ParseInLocation("2006-01-02", *from, location)
	if err != nil {
		return fmt.Errorf("invalid --from %q; expected YYYY-MM-DD: %w", *from, err)
	}
	toDate, err := time.ParseInLocation("2006-01-02", *to, location)
	if err != nil {
		return fmt.Errorf("invalid --to %q; expected YYYY-MM-DD: %w", *to, err)
	}
	if fromDate.After(toDate) {
		return errors.New("--from must be on or before --to")
	}
	body := map[string]any{
		"fromPostingDateTs": fromDate.UnixMilli(),
		"toPostingDateTs":   toDate.AddDate(0, 0, 1).UnixMilli() - 1,
	}
	if *fiscalYear != 0 {
		body["fiscalYear"] = *fiscalYear
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	raw, err := client.Bytes(http.MethodPost, "/datevexport", bytes.NewReader(rawBody), map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}, nil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(cliOutput, *out)
	return nil
}

func datevImport(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("datev import", flag.ContinueOnError)
	flags.SetOutput(cliError)
	file := flags.String("file", "", "DATEV EXTF input file")
	dryRun := flags.Bool("dry-run", false, "read and preview metadata only; no API request")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, datevImportUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *file == "" {
		flags.Usage()
		return errors.New("datev import requires --file")
	}

	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	preview := map[string]any{
		"endpoint": "POST /datevpostings/new",
		"file":     *file,
		"bytes":    len(raw),
	}
	req := writeRequest{
		Command:       "datev import",
		Method:        http.MethodPost,
		Path:          "/datevpostings/new",
		Preview:       preview,
		ConfirmPhrase: "import datev " + filepath.Base(*file),
	}
	writePreview(client, req)
	if *dryRun {
		preview["status"] = "dry_run"
		return printJSON(preview)
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	payload := datevImportPayload{Data: base64.StdEncoding.EncodeToString(raw)}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	response, err := client.Bytes(http.MethodPost, req.Path, bytes.NewReader(rawBody), map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json",
	}, nil)
	if err != nil {
		return err
	}
	if len(response) == 0 {
		return nil
	}
	_, err = cliOutput.Write(response)
	return err
}
