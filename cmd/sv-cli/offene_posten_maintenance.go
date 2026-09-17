package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const offenePostenRebookUsage = `usage: sv-cli offene-posten rebook --seite=debitor|kreditor --data @rebook.json [--dry-run] [--yes]

Input schema (--data JSON or @file):
  documentNumber  Belegnummer
  postingDate     Buchungsdatum
  postingPeriod   optional period number
  sourceNumber    source Personenkonto number
  targetNumber    target Personenkonto number

The command sends exactly one rebooking request after confirmation.`

const offenePostenSetReminderLevelUsage = `usage: sv-cli offene-posten set-reminder-level --data @reminder-levels.json [--dry-run] [--yes]

Input schema (--data JSON or @file):
  openItems  array of {documentNumber, reminderLevel}

The command sends exactly one reminder-level request after confirmation.
Per-item failures produce a non-zero exit.`

func offenePostenRebook(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("offene-posten rebook", flag.ContinueOnError)
	flags.SetOutput(cliError)
	seiteFlag := flags.String("seite", "", "required side: debitor or kreditor")
	data := flags.String("data", "", "rebooking JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, offenePostenRebookUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *data == "" {
		flags.Usage()
		return errors.New("offene-posten rebook requires --data and takes no positional arguments")
	}
	seite, err := parseOffenePostenSeite(*seiteFlag)
	if err != nil {
		return err
	}
	payload, err := loadJSONObject(*data)
	if err != nil {
		return err
	}
	if payload == nil {
		return errors.New("--data must contain a JSON object")
	}
	confirmPhrase := "rebook offene posten"
	if documentNumber, ok := payload["documentNumber"].(string); ok && documentNumber != "" {
		confirmPhrase = "rebook " + documentNumber
	}
	req := writeRequest{
		Command:       "offene-posten rebook",
		Method:        http.MethodPost,
		Path:          seite.rebookEndpoint,
		Payload:       payload,
		ConfirmPhrase: confirmPhrase,
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST " + req.Path,
			"request":  payload,
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
		"status":   "rebooked",
		"endpoint": "POST " + req.Path,
		"response": result,
	})
}

func offenePostenSetReminderLevel(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("offene-posten set-reminder-level", flag.ContinueOnError)
	flags.SetOutput(cliError)
	data := flags.String("data", "", "reminder-level JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, offenePostenSetReminderLevelUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *data == "" {
		flags.Usage()
		return errors.New("offene-posten set-reminder-level requires --data and takes no positional arguments")
	}
	payload, err := loadJSONObject(*data)
	if err != nil {
		return err
	}
	if payload == nil {
		return errors.New("--data must contain a JSON object")
	}
	req := writeRequest{
		Command:       "offene-posten set-reminder-level",
		Method:        http.MethodPost,
		Path:          "/openitems/setReminderLevel",
		Payload:       payload,
		ConfirmPhrase: reminderLevelConfirmPhrase(payload),
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST " + req.Path,
			"request":  payload,
		})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, _, err := executeWriteOnce(client, req, func(any) bool { return true })
	if err != nil {
		return err
	}
	failures, err := reminderLevelFailures(result)
	output := map[string]any{
		"endpoint": "POST " + req.Path,
		"response": result,
	}
	if err != nil {
		output["status"] = "verification_required"
		if printErr := printJSON(output); printErr != nil {
			return printErr
		}
		return err
	}
	if len(failures) != 0 {
		output["status"] = "partial_failure"
		output["failures"] = failures
		if err := printJSON(output); err != nil {
			return err
		}
		return fmt.Errorf("reminder level update failed for %s", reminderLevelFailureSummary(failures))
	}
	output["status"] = "updated"
	return printJSON(output)
}

func reminderLevelConfirmPhrase(payload map[string]any) string {
	if items, ok := payload["openItems"].([]any); ok {
		return fmt.Sprintf("set reminder levels for %d open items", len(items))
	}
	return "set reminder levels"
}

func reminderLevelFailures(result any) ([]map[string]any, error) {
	response, ok := result.(map[string]any)
	if !ok {
		return nil, errors.New("reminder-level response is not a JSON object")
	}
	items, ok := response["openItems"].([]any)
	if !ok {
		return nil, errors.New("reminder-level response has no openItems array")
	}
	failures := make([]map[string]any, 0)
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("reminder-level response openItems[%d] is not an object", i)
		}
		success, ok := item["success"].(bool)
		if !ok {
			return nil, fmt.Errorf("reminder-level response openItems[%d] has no boolean success", i)
		}
		if !success {
			failures = append(failures, item)
		}
	}
	return failures, nil
}

func reminderLevelFailureSummary(failures []map[string]any) string {
	summaries := make([]string, 0, len(failures))
	for _, failure := range failures {
		documentNumber, _ := failure["documentNumber"].(string)
		message, _ := failure["message"].(string)
		summary := documentNumber
		if summary == "" {
			summary = "unknown document"
		}
		if message != "" {
			summary += ": " + message
		}
		summaries = append(summaries, summary)
	}
	return strings.Join(summaries, "; ")
}
