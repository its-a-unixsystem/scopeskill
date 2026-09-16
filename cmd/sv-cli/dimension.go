package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var dimensionSearchCommand = searchEndpointCommand{
	name:     "dimension search",
	endpoint: "/dimensions",
}

func dimension(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("missing dimension subcommand")
	}
	switch args[0] {
	case "search":
		return runSearchEndpointCommand(client, dimensionSearchCommand, args[1:])
	case "entries":
		return dimensionEntries(client, args[1:])
	case "entry":
		return dimensionEntry(client, args[1:])
	default:
		return fmt.Errorf("unknown dimension command: %s", args[0])
	}
}

func dimensionEntry(client *scopeskill.Client, args []string) error {
	if len(args) > 0 && (args[0] == "create" || args[0] == "update") {
		return dimensionEntryWrite(client, args[1:], args[0])
	}
	return errors.New("dimension entry subcommand must be create or update")
}

func dimensionEntryWrite(client *scopeskill.Client, args []string, operation string) error {
	flags := flag.NewFlagSet("dimension entry "+operation, flag.ContinueOnError)
	flags.SetOutput(cliError)
	number := flags.Int64("number", 0, "dimension entry number")
	entryName := flags.String("name", "", "dimension entry name")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip confirmation")
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *number == 0 || *entryName == "" {
		return errors.New("dimension entry requires <dimension>, --number and --name")
	}
	suffix, status := "/dimensionentry", "updated"
	if operation == "create" {
		suffix, status = "/dimensionentry/new", "created"
	}
	payload := map[string]any{"number": *number, "name": *entryName, "locked": false}
	path := "/dimensions/" + url.PathEscape(flags.Arg(0)) + suffix
	req := writeRequest{Command: "dimension entry " + operation, Method: http.MethodPost, Path: path, Payload: payload, ConfirmPhrase: operation + " dimension entry"}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{"status": "dry_run", "endpoint": "POST " + path, "request": payload})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, outcome, err := executeWriteOnce(client, req, func(any) bool { return true })
	if err != nil {
		return err
	}
	if outcome != writeAccepted {
		status = "verification_required"
	}
	return printJSON(map[string]any{"status": status, "endpoint": "POST " + path, "response": result})
}

func dimensionEntries(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("dimension entries", flag.ContinueOnError)
	flags.SetOutput(cliError)
	page := flags.Int("page", -1, "page number")
	pageSize := flags.Int("page-size", 0, "page size")
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli dimension entries <dimension> [--page=N] [--page-size=N]")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("dimension entries takes exactly one dimension")
	}
	if *page < -1 {
		return errors.New("--page must be non-negative")
	}
	if *pageSize < 0 || *pageSize > scopeskill.MaxSearchPageSize {
		return fmt.Errorf("--page-size must be between 1 and %d", scopeskill.MaxSearchPageSize)
	}
	query := map[string]string{}
	if *page >= 0 {
		query["page"] = fmt.Sprint(*page)
	}
	if *pageSize > 0 {
		query["pageSize"] = fmt.Sprint(*pageSize)
	}
	raw, err := client.JSON(http.MethodGet, "/dimensions/"+url.PathEscape(flags.Arg(0))+"/dimensionentries", nil, query)
	if err != nil {
		return err
	}
	return printJSON(raw)
}

func textbaustein(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "textbaustein subcommands: list")
		return errors.New("missing textbaustein subcommand")
	}
	switch args[0] {
	case "list":
		return textbausteinList(client, args[1:])
	default:
		return fmt.Errorf("unknown textbaustein command: %s", args[0])
	}
}

func textbausteinList(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("textbaustein list", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli textbaustein list")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("textbaustein list takes no positional arguments")
	}
	raw, err := client.JSON(http.MethodGet, "/texttemplates", nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
