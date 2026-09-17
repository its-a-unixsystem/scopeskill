package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const kasseCreateUsage = `usage: sv-cli kasse create --data JSON|@entry.json [--dry-run] [--yes]

Creates a Kassenbuchung via POST /cashbookentry/new.

Input is a CashbookEntryForm JSON object. cashbookId and documentDate (epoch
milliseconds) are required. internalDocumentNumber is read-only and rejected.

Safety:
  --dry-run   preview only; no write
  --yes       bypass the interactive confirmation`

func kasse(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "kasse subcommands: list create")
		return errors.New("missing kasse subcommand")
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintln(cliOutput, "kasse subcommands: list create")
		return nil
	case "list":
		return runSearchEndpointCommand(client, searchEndpointCommand{
			name:     "kasse list",
			endpoint: "/cashbooksheets",
		}, args[1:])
	case "create":
		return kasseCreate(client, args[1:])
	default:
		return fmt.Errorf("unknown kasse command: %s", args[0])
	}
}

func kasseCreate(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("kasse create", flag.ContinueOnError)
	flags.SetOutput(cliError)
	data := flags.String("data", "", "CashbookEntryForm JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, kasseCreateUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if *data == "" || flags.NArg() != 0 {
		flags.Usage()
		return errors.New("kasse create requires --data and takes no positional arguments")
	}

	raw, err := loadRaw(*data)
	if err != nil {
		return err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return errors.New("--data must contain a JSON object")
	}
	cashbookID, err := requiredJSONInt64(body, "cashbookId")
	if err != nil {
		return err
	}
	if _, err := requiredJSONInt64(body, "documentDate"); err != nil {
		return err
	}
	if _, present := body["internalDocumentNumber"]; present {
		return errors.New("internalDocumentNumber is read-only and must not be sent")
	}

	req := writeRequest{
		Command:       "kasse create",
		Method:        http.MethodPost,
		Path:          "/cashbookentry/new",
		Payload:       json.RawMessage(raw),
		ConfirmPhrase: fmt.Sprintf("create kassenbuchung %d", cashbookID),
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST /cashbookentry/new",
			"request":  req.Payload,
		})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, _, err := executeWriteOnce(client, req, func(any) bool { return true })
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"status":   "created",
		"endpoint": "POST /cashbookentry/new",
		"response": result,
	})
}

func requiredJSONInt64(body map[string]json.RawMessage, field string) (int64, error) {
	raw, present := body[field]
	if !present {
		return 0, fmt.Errorf("%s is required", field)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("%s must be an int64", field)
	}
	return value, nil
}
