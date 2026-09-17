package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const reisekostenHelp = "reisekosten subcommands: list nebenkosten uebernachtung fahrtkosten"

func reisekosten(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, reisekostenHelp)
		return errors.New("missing reisekosten subcommand")
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprintln(cliOutput, reisekostenHelp)
		return nil
	case "list":
		return runSearchEndpointCommand(client, searchEndpointCommand{
			name:     "reisekosten list",
			endpoint: "/travelentries",
		}, args[1:])
	case "nebenkosten":
		return reisekostenPosition(client, args[1:], "nebenkosten", "/travelentry/position/extra/new")
	case "uebernachtung":
		return reisekostenPosition(client, args[1:], "uebernachtung", "/travelentry/position/overnight/new")
	case "fahrtkosten":
		return reisekostenPosition(client, args[1:], "fahrtkosten", "/travelentry/position/vehicle/new")
	default:
		return fmt.Errorf("unknown reisekosten command: %s", args[0])
	}
}

func reisekostenPosition(client *scopeskill.Client, args []string, kind, endpoint string) error {
	help := func() {
		fmt.Fprintf(cliOutput, "reisekosten %s subcommands: create\n", kind)
	}
	if len(args) == 0 {
		help()
		return fmt.Errorf("reisekosten %s subcommand must be create", kind)
	}
	switch args[0] {
	case "help", "-h", "--help":
		help()
		return nil
	case "create":
		return reisekostenPositionCreate(client, args[1:], kind, endpoint)
	default:
		return fmt.Errorf("reisekosten %s subcommand must be create", kind)
	}
}

func reisekostenPositionCreate(client *scopeskill.Client, args []string, kind, endpoint string) error {
	flags := flag.NewFlagSet("reisekosten "+kind+" create", flag.ContinueOnError)
	flags.SetOutput(cliError)
	file := flags.String("file", "", "provider JSON object file")
	dryRun := flags.Bool("dry-run", false, "preview the request without writing")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli reisekosten %s create --file=position.json [--dry-run] [--yes]\n\n", kind)
		fmt.Fprintf(cliError, "Posts to %s. The file must be a provider JSON object containing travelEntryId and fileform.\n", endpoint)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if *file == "" || flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("reisekosten %s create requires --file and takes no positional arguments", kind)
	}

	raw, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("--file must contain a JSON object")
	}

	req := writeRequest{
		Command:       "reisekosten " + kind + " create",
		Method:        http.MethodPost,
		Path:          endpoint,
		Payload:       json.RawMessage(raw),
		ConfirmPhrase: "create " + kind + " position " + filepath.Base(*file),
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST " + endpoint,
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
		"endpoint": "POST " + endpoint,
		"response": result,
	})
}
