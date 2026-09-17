package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const (
	maxIncomingInvoiceSizeBytes = 20 * 1024 * 1024 // 20 MB API limit
	eingangsrechnungImportUsage = `usage: sv-cli eingangsrechnung import --file=invoice.pdf [--dry-run] [--yes]

Imports a vendor invoice PDF via POST /incominginvoice/new.

Safety:
  --file=PATH   path to PDF file (max 20 MB)
  --dry-run     preview the upload payload only; no write
  --yes         bypass the interactive "import eingangsrechnung <filename>" confirmation`
)

func eingangsrechnungImport(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("eingangsrechnung import", flag.ContinueOnError)
	flags.SetOutput(cliError)
	file := flags.String("file", "", "path to PDF file")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, eingangsrechnungImportUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *file == "" {
		flags.Usage()
		return errors.New("eingangsrechnung import requires --file")
	}

	info, err := os.Stat(*file)
	if err != nil {
		return err
	}
	if info.Size() > maxIncomingInvoiceSizeBytes {
		return fmt.Errorf("file size (%d bytes) exceeds 20 MB limit", info.Size())
	}

	bytes, err := os.ReadFile(*file)
	if err != nil {
		return err
	}

	filename := filepath.Base(*file)
	encoded := base64.StdEncoding.EncodeToString(bytes)

	payload := map[string]any{
		"filename": filename,
		"data":     encoded,
	}
	preview := map[string]any{
		"filename":       filename,
		"fileSizeBytes":  len(bytes),
		"encodedDataLen": len(encoded),
	}

	req := writeRequest{
		Command:       "eingangsrechnung import",
		Method:        http.MethodPost,
		Path:          "/incominginvoice/new",
		Payload:       payload,
		Preview:       preview,
		ConfirmPhrase: "import eingangsrechnung " + filename,
	}

	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"filename": filename,
			"size":     len(bytes),
			"endpoint": "POST /incominginvoice/new",
		})
	}

	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}

	result, outcome, err := executeWriteOnce(client, req, func(result any) bool { return true })
	if outcome == writeRejected || err != nil {
		return err
	}

	out := map[string]any{
		"status":   "imported",
		"filename": filename,
		"size":     len(bytes),
	}
	if result != nil {
		out["result"] = result
	}
	return printJSON(out)
}
