package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const buchungFileAddUsage = `usage: sv-cli buchung file add <documentNumber> <file> [--dry-run] [--yes]

Attaches <file> to the Buchung using POST /journal/<documentNumber>/file/new.

Safety:
  --dry-run   preview the FileForm payload only; no write
  --yes       bypass the interactive "attach <documentNumber>" confirmation`

const buchungFileGetUsage = `usage: sv-cli buchung file get <documentNumber> [--out <file>] [--with-stamp]

Downloads the attached Beleg. Without --out, the response filename is used.
--with-stamp retrieves /journal/<documentNumber>/filewithstamp.`

type journalFileForm struct {
	Filename string `json:"filename"`
	Data     string `json:"data"`
}

func buchungFile(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "buchung file subcommands: add get")
		return errors.New("missing buchung file subcommand")
	}
	switch args[0] {
	case "add":
		return buchungFileAdd(client, args[1:])
	case "get":
		return buchungFileGet(client, args[1:])
	default:
		return fmt.Errorf("unknown buchung file command: %s", args[0])
	}
}

func buchungFileAdd(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung file add", flag.ContinueOnError)
	flags.SetOutput(cliError)
	dryRun := flags.Bool("dry-run", false, "preview the FileForm payload only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungFileAddUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		flags.Usage()
		return errors.New("buchung file add takes exactly one documentNumber and one file")
	}

	documentNumber := flags.Arg(0)
	filePath := flags.Arg(1)
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("cannot attach empty file %q", filePath)
	}
	payload := journalFileForm{
		Filename: filepath.Base(filePath),
		Data:     base64.StdEncoding.EncodeToString(raw),
	}
	req := writeRequest{
		Command:       "buchung file add",
		Method:        http.MethodPost,
		Path:          "/journal/" + url.PathEscape(documentNumber) + "/file/new",
		Payload:       payload,
		ConfirmPhrase: "attach " + documentNumber,
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(journalFileStatus("dry_run", documentNumber, payload.Filename))
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome != writeAccepted {
		if writeErr != nil {
			return writeErr
		}
		return errors.New("file attachment response was ambiguous")
	}
	output := journalFileStatus("added", documentNumber, payload.Filename)
	if result != nil {
		output["response"] = result
	}
	return printJSON(output)
}

func buchungFileGet(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung file get", flag.ContinueOnError)
	flags.SetOutput(cliError)
	out := flags.String("out", "", "output file path; defaults to the response filename")
	withStamp := flags.Bool("with-stamp", false, "include configured invoice stamps")
	flags.Usage = func() { fmt.Fprintln(cliError, buchungFileGetUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("buchung file get takes exactly one documentNumber")
	}

	documentNumber := flags.Arg(0)
	suffix := "/file"
	if *withStamp {
		suffix = "/filewithstamp"
	}
	path, err := client.DownloadNamed("/journal/"+url.PathEscape(documentNumber)+suffix, *out, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(cliOutput, path)
	return nil
}

func journalFileStatus(status, documentNumber, filename string) map[string]any {
	return map[string]any{
		"status":         status,
		"documentNumber": documentNumber,
		"filename":       filename,
	}
}
