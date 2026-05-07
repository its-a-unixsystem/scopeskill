package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var kontaktSearchDefaultFields = []string{
	"id", "lastname", "companyname", "firstname",
	"email", "vatId",
	"debitorNumber", "kreditorNumber",
}

func kontakt(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "kontakt subcommands: search show")
		return errors.New("missing kontakt subcommand")
	}
	switch args[0] {
	case "search":
		return kontaktSearch(client, args[1:])
	case "show":
		return kontaktShow(client, args[1:])
	default:
		return fmt.Errorf("unknown kontakt command: %s", args[0])
	}
}

const kontaktSearchUsage = `usage: sv-cli kontakt search [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters:
  --name=SUBSTRING        lastname contains
  --ust-id=VALUE          vatId equals
  --email=SUBSTRING       email contains

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

func kontaktSearch(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("kontakt search", flag.ContinueOnError)
	flags.SetOutput(cliError)
	name := flags.String("name", "", "filter: lastname contains substring")
	ustID := flags.String("ust-id", "", "filter: vatId equals")
	email := flags.String("email", "", "filter: email contains substring")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, kontaktSearchUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("kontakt search takes no positional arguments")
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
		raw, err := client.JSON(http.MethodPost, "/contacts", body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, kontaktSearchDefaultFields...),
		Order:  []string{"lastname = asc"},
	}
	if *name != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "lastname", Operator: scopeskill.OpContains, Value: *name,
		})
	}
	if *ustID != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "vatId", Operator: scopeskill.OpEquals, Value: *ustID,
		})
	}
	if *email != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "email", Operator: scopeskill.OpContains, Value: *email,
		})
	}

	fetch := func(body map[string]any) ([]any, error) {
		raw, err := client.JSON(http.MethodPost, "/contacts", body, nil)
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

func kontaktShow(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("kontakt show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli kontakt show <id>")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("kontakt show takes exactly one Kontakt id")
	}
	id := flags.Arg(0)

	kontakt, err := scopeskill.FetchKontaktByID(client, id)
	if err != nil {
		return err
	}
	if kontakt == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}

	out := map[string]any{"kontakt": kontakt}
	if num := nonEmptyString(kontakt["debitorNumber"]); num != "" {
		out["debitor"] = map[string]any{"number": num}
	}
	if num := nonEmptyString(kontakt["kreditorNumber"]); num != "" {
		out["kreditor"] = map[string]any{"number": num}
	}
	return printJSON(out)
}

func nonEmptyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%g", v)
	default:
		return ""
	}
}
