package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var sachkontoSearchDefaultFields = []string{"id", "number", "name", "active", "accountTypeName"}

// nowFunc is the test seam for "today"; production uses time.Now.
var nowFunc = time.Now

const isoDateFormat = "2006-01-02"

const notFoundOrUnauthorisedMessage = "not found or authorization missing"

func sachkonto(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "sachkonto subcommands: search show balance")
		return errors.New("missing sachkonto subcommand")
	}
	switch args[0] {
	case "search":
		return sachkontoSearch(client, args[1:])
	case "show":
		return sachkontoShow(client, args[1:])
	case "balance":
		return sachkontoBalance(client, args[1:])
	default:
		return fmt.Errorf("unknown sachkonto command: %s", args[0])
	}
}

// fetchSachkontoByNumber resolves one Sachkonto by Kontonummer. Returns nil
// when no record matches.
func fetchSachkontoByNumber(client *scopeskill.Client, number string) (map[string]any, error) {
	req := scopeskill.SearchRequest{
		PageSize: 1,
		Fields:   append([]string{}, sachkontoSearchDefaultFields...),
		Conditions: []scopeskill.SearchCondition{
			{Field: "number", Operator: scopeskill.OpEquals, Value: number},
		},
	}
	body, err := req.Body()
	if err != nil {
		return nil, err
	}
	raw, err := client.JSON(http.MethodPost, "/impersonalaccounts", body, nil)
	if err != nil {
		return nil, err
	}
	records, err := scopeskill.RecordsFromResponse(raw)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	rec, _ := records[0].(map[string]any)
	return rec, nil
}

func sachkontoShow(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("sachkonto show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli sachkonto show <Kontonummer>")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("sachkonto show takes exactly one Kontonummer")
	}
	number := flags.Arg(0)

	konto, err := fetchSachkontoByNumber(client, number)
	if err != nil {
		return err
	}
	if konto == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}

	years, err := scopeskill.FetchFiscalYears(client)
	if err != nil {
		return err
	}
	now := nowFunc()
	current, hasCurrent := scopeskill.CurrentFiscalYear(years, now)
	earliest, hasEarliest := scopeskill.EarliestFiscalYear(years)
	if !hasCurrent || !hasEarliest {
		return errors.New("no fiscal year information available; cannot derive Saldo range")
	}

	saldoCurrent, err := scopeskill.FetchSaldo(client, scopeskill.SaldoEndpointSachkonto, number, earliest.Beginning, now)
	if err != nil {
		return err
	}
	saldoFYtD, err := scopeskill.FetchSaldo(client, scopeskill.SaldoEndpointSachkonto, number, current.Beginning, now)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		"konto": konto,
		"saldo": map[string]any{
			"current":          saldoCurrent,
			"fiscalYearToDate": saldoFYtD,
		},
	})
}

func sachkontoBalance(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("sachkonto balance", flag.ContinueOnError)
	flags.SetOutput(cliError)
	from := flags.String("from", "", "start date (yyyy-mm-dd); default = current fiscal year start")
	to := flags.String("to", "", "end date (yyyy-mm-dd); default = today")
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli sachkonto balance <Kontonummer> [--from=YYYY-MM-DD] [--to=YYYY-MM-DD]")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("sachkonto balance takes exactly one Kontonummer")
	}
	number := flags.Arg(0)

	now := nowFunc()
	fromDate, toDate, err := resolveSaldoRange(client, *from, *to, now)
	if err != nil {
		return err
	}

	rec, err := scopeskill.FetchSaldo(client, scopeskill.SaldoEndpointSachkonto, number, fromDate, toDate)
	if err != nil {
		return err
	}
	if rec == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}
	return printJSON(rec)
}

func resolveSaldoRange(client *scopeskill.Client, from, to string, now time.Time) (time.Time, time.Time, error) {
	toDate := now
	if to != "" {
		t, err := time.Parse(isoDateFormat, to)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--to: %w", err)
		}
		toDate = t
	}
	if from != "" {
		t, err := time.Parse(isoDateFormat, from)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--from: %w", err)
		}
		return t, toDate, nil
	}
	years, err := scopeskill.FetchFiscalYears(client)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	current, ok := scopeskill.CurrentFiscalYear(years, now)
	if !ok {
		return time.Time{}, time.Time{}, errors.New("no current fiscal year; pass --from explicitly")
	}
	return current.Beginning, toDate, nil
}

const sachkontoSearchUsage = `usage: sv-cli sachkonto search [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters (each maps to a single Scopevisio search condition):
  --name=SUBSTRING        Bezeichnung contains
  --number-prefix=PREFIX  Kontonummer starts with (field: number)
  --active                only active Konten (active equals true)
  --type=NAME             accountTypeName equals

Pagination:
  default                 single page at pageSize=100
  --page-size=N           override the single-page pageSize (1..1000)
  --all                   page through all results at pageSize=1000, capped at 10000
  --max=N                 raise the --all safety cap (default 10000)

Escape hatch:
  --data @file.json       full search-body override; cannot combine with --all,
                          --page-size, or --max.

Output is JSON on stdout: an array of records (or the raw API response when
--data is used).`

func sachkontoSearch(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("sachkonto search", flag.ContinueOnError)
	flags.SetOutput(cliError)
	name := flags.String("name", "", "filter: Bezeichnung contains substring")
	numberPrefix := flags.String("number-prefix", "", "filter: Kontonummer starts with prefix")
	activeOnly := flags.Bool("active", false, "filter: active = true")
	accountType := flags.String("type", "", "filter: accountTypeName equals")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, sachkontoSearchUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("sachkonto search takes no positional arguments")
	}
	if *data != "" && (*all || *pageSize != 0 || *max != 0) {
		return errors.New("--data cannot be combined with --all, --page-size, or --max")
	}
	if *pageSize < 0 || *pageSize > scopeskill.MaxSearchPageSize {
		return fmt.Errorf("--page-size must be between 1 and %d", scopeskill.MaxSearchPageSize)
	}
	if *max < 0 {
		return errors.New("--max must be non-negative")
	}

	if *data != "" {
		body, err := loadJSONObject(*data)
		if err != nil {
			return err
		}
		raw, err := client.JSON(http.MethodPost, "/impersonalaccounts", body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, sachkontoSearchDefaultFields...),
		Order:  []string{"number = asc"},
	}
	if *name != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "name", Operator: scopeskill.OpContains, Value: *name,
		})
	}
	if *numberPrefix != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "number", Operator: scopeskill.OpStartsWith, Value: *numberPrefix,
		})
	}
	if *activeOnly {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "active", Operator: scopeskill.OpEquals, Value: true,
		})
	}
	if *accountType != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "accountTypeName", Operator: scopeskill.OpEquals, Value: *accountType,
		})
	}

	fetch := func(body map[string]any) ([]any, error) {
		raw, err := client.JSON(http.MethodPost, "/impersonalaccounts", body, nil)
		if err != nil {
			return nil, err
		}
		return scopeskill.RecordsFromResponse(raw)
	}

	records, err := scopeskill.Paginate(scopeskill.PaginateOptions{
		All:      *all,
		PageSize: *pageSize,
		Max:      *max,
	}, base, fetch)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}
