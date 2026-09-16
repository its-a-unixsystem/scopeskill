package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungCorrectUsage = `usage: sv-cli buchung correct <documentNumber> --file=correction.json [--dry-run] [--yes]

Submits a correction posting via POST /correctpostings. The file uses the same
single-Buchung JSON schema as "buchung create".`

const buchungCorrectImportUsage = `usage: sv-cli buchung correct-import --file=corrections.json [--dry-run] [--yes]

Submits an array of correction postings via POST /postings/correction. Each
array element uses the same single-Buchung JSON schema as "buchung create".`

func buchungCorrect(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung correct", flag.ContinueOnError)
	flags.SetOutput(cliError)
	file := flags.String("file", "", "correction JSON, or @path")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungCorrectUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" || *file == "" {
		flags.Usage()
		return errors.New("buchung correct requires one documentNumber and --file")
	}
	raw, err := loadRaw(*file)
	if err != nil {
		return err
	}
	in, err := scopeskill.ParseSinglePostingInput(raw)
	if err != nil {
		return err
	}
	if in.DocumentNumber != flags.Arg(0) {
		return fmt.Errorf("documentNumber %q in file does not match %q", in.DocumentNumber, flags.Arg(0))
	}
	payload, err := in.ToPostingsRequest()
	if err != nil {
		return err
	}
	req := writeRequest{Command: "buchung correct", Method: http.MethodPost, Path: "/correctpostings", Payload: payload, ConfirmPhrase: "correct " + flags.Arg(0)}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{"status": "dry_run", "documentNumber": in.DocumentNumber, "endpoint": "POST /correctpostings", "request": payload})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome != writeAccepted {
		if writeErr != nil {
			return writeErr
		}
		return errors.New("correction write requires verification")
	}
	out := map[string]any{"status": "corrected", "documentNumber": in.DocumentNumber, "endpoint": "POST /correctpostings", "request": payload}
	if result != nil {
		out["response"] = result
	}
	return printJSON(out)
}

func buchungCorrectImport(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung correct-import", flag.ContinueOnError)
	flags.SetOutput(cliError)
	file := flags.String("file", "", "corrections JSON file")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungCorrectImportUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *file == "" {
		flags.Usage()
		return errors.New("buchung correct-import requires --file and takes no positional arguments")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var inputs []scopeskill.SinglePostingInput
	// Decode each item through the strict, established single-posting parser.
	var raws []json.RawMessage
	if err := json.Unmarshal(raw, &raws); err != nil {
		return err
	}
	for _, item := range raws {
		in, err := scopeskill.ParseSinglePostingInput(item)
		if err != nil {
			return err
		}
		inputs = append(inputs, in)
	}
	payload := make([]scopeskill.PostingsRequest, 0, len(inputs))
	for _, in := range inputs {
		p, err := in.ToPostingsRequest()
		if err != nil {
			return err
		}
		payload = append(payload, p)
	}
	req := writeRequest{Command: "buchung correct-import", Method: http.MethodPost, Path: "/postings/correction", Payload: payload, ConfirmPhrase: "correct-import " + *file}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{"status": "dry_run", "endpoint": "POST /postings/correction", "request": payload})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome != writeAccepted {
		if writeErr != nil {
			return writeErr
		}
		return errors.New("correction import requires verification")
	}
	out := map[string]any{"status": "corrected", "endpoint": "POST /postings/correction", "count": len(payload)}
	if result != nil {
		out["response"] = result
	}
	return printJSON(out)
}
