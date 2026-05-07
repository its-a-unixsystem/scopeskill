package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var sachkontoSearchDefaultFields = []string{"id", "number", "name", "active", "accountTypeName"}

func sachkonto(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "sachkonto subcommands: search")
		return errors.New("missing sachkonto subcommand")
	}
	switch args[0] {
	case "search":
		return sachkontoSearch(client, args[1:])
	default:
		return fmt.Errorf("unknown sachkonto command: %s", args[0])
	}
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
