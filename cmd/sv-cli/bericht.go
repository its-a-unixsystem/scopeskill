package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

const (
	berichtShowUsage    = `usage: sv-cli bericht show --type=bwa|bilanz|guv --name=NAME --year=YYYY --month=MM [--show-accounts] [--layout=NAME]`
	defaultReportLayout = "Standard"
)

var proReportTypes = map[string]string{
	"guv":    "1",
	"bilanz": "2",
	"bwa":    "3",
}

func bericht(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "bericht subcommands: show export")
		return nil
	}
	switch args[0] {
	case "show":
		return berichtShow(client, args[1:])
	case "export":
		return berichtExport(client, args[1:])
	default:
		return fmt.Errorf("unknown bericht command: %s", args[0])
	}
}

func berichtShow(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("bericht show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	reportType := flags.String("type", "", "report type: bwa, bilanz, or guv")
	name := flags.String("name", "", "account layout name")
	year := flags.Int("year", 0, "report year")
	month := flags.Int("month", 0, "report month (1-12)")
	showAccounts := flags.Bool("show-accounts", false, "include accounts")
	layout := flags.String("layout", defaultReportLayout, "column layout name")
	flags.Usage = func() { fmt.Fprintln(cliError, berichtShowUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *reportType == "" || *name == "" || *year == 0 || *month == 0 {
		return errors.New(berichtShowUsage)
	}
	apiType, err := proReportType(*reportType)
	if err != nil {
		return err
	}
	if *year < 1 {
		return errors.New("--year must be positive")
	}
	if *month < 1 || *month > 12 {
		return errors.New("--month must be between 1 and 12")
	}

	query := map[string]string{"layoutName": *layout}
	if *showAccounts {
		query["showAccounts"] = "true"
	}
	path := fmt.Sprintf("/proreport/%s/%s/%d/%d", apiType, url.PathEscape(*name), *year, *month)
	raw, err := client.Bytes(http.MethodGet, path, nil, map[string]string{"Accept": "application/json"}, query)
	if err != nil {
		return err
	}
	_, err = cliOutput.Write(raw)
	return err
}

const berichtExportUsage = `usage: sv-cli bericht export --type=bwa|bilanz|guv --from=DD.MM.YYYY --to=DD.MM.YYYY [--format=csv|pdf] [--out=FILE]`

func berichtExport(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("bericht export", flag.ContinueOnError)
	flags.SetOutput(cliError)
	reportType := flags.String("type", "", "report type: bwa, bilanz, or guv")
	from := flags.String("from", "", "first report date (DD.MM.YYYY)")
	to := flags.String("to", "", "last report date (DD.MM.YYYY)")
	format := flags.String("format", "csv", "output format: csv or pdf")
	out := flags.String("out", "", "output file path")
	flags.Usage = func() { fmt.Fprintln(cliError, berichtExportUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 || *reportType == "" || *from == "" || *to == "" {
		return errors.New(berichtExportUsage)
	}
	if _, err := proReportType(*reportType); err != nil {
		return err
	}
	if *format != "csv" && *format != "pdf" {
		return fmt.Errorf("invalid --format %q; expected csv or pdf", *format)
	}
	fromDate, err := time.Parse("02.01.2006", *from)
	if err != nil {
		return fmt.Errorf("invalid --from %q; expected DD.MM.YYYY: %w", *from, err)
	}
	toDate, err := time.Parse("02.01.2006", *to)
	if err != nil {
		return fmt.Errorf("invalid --to %q; expected DD.MM.YYYY: %w", *to, err)
	}
	if fromDate.After(toDate) {
		return errors.New("--from must be on or before --to")
	}

	query := map[string]string{
		"startDate":    *from,
		"endDate":      *to,
		"outputFormat": *format,
	}
	written, err := client.DownloadNamed("/reports/"+*reportType, *out, query)
	if err != nil {
		return err
	}
	fmt.Fprintln(cliOutput, written)
	return nil
}

func proReportType(value string) (string, error) {
	apiType, ok := proReportTypes[value]
	if !ok {
		return "", fmt.Errorf("invalid --type %q; expected bwa, bilanz, or guv", value)
	}
	return apiType, nil
}
