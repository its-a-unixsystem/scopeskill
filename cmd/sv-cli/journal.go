package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var journalSearchDefaultFields = []string{
	"documentNumber",
	"postingDate",
	"documentText",
	"postingText",
	"accountNumber",
	"accountName",
	"debitAmount",
	"creditAmount",
	"amount",
	"vatKey",
	"vatRate",
	"internalDocumentNumber",
	"externalDocumentNumber",
	"documentDimension_1",
	"documentDimension_2",
	"documentDimension_3",
	"documentDimension_4",
	"documentDimension_5",
	"documentDimension_6",
	"documentDimension_7",
	"documentDimension_8",
	"documentDimension_9",
	"documentDimension_10",
}

func journal(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "journal subcommands: search")
		return errors.New("missing journal subcommand")
	}
	switch args[0] {
	case "search":
		return journalSearch(client, args[1:])
	default:
		return fmt.Errorf("unknown journal command: %s", args[0])
	}
}

func buchung(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "buchung subcommands: show")
		return errors.New("missing buchung subcommand")
	}
	switch args[0] {
	case "show":
		return buchungShow(client, args[1:])
	default:
		return fmt.Errorf("unknown buchung command: %s", args[0])
	}
}

const journalSearchUsage = `usage: sv-cli journal search [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters:
  --from=YYYY-MM-DD        postingDate on or after date
  --to=YYYY-MM-DD          postingDate on or before date
  --konto=NUMBER           accountNumber equals
  --text=SUBSTRING         postingText contains
  --belegnr=NUMBER         documentNumber equals
  --amount-min=AMOUNT      amount greater than or equal
  --amount-max=AMOUNT      amount less than or equal
  --dim=KEY=VALUE          dimension filter; repeatable

Dimension keys:
  kostenstelle             documentDimension_1
  kostentraeger            documentDimension_2
  projekt                  documentDimension_3
  dimension_N              documentDimension_N

Pagination:
  default                  single page at pageSize=100
  --page-size=N            override the single-page pageSize (1..1000)
  --all                    page through all results at pageSize=1000, capped at 10000
  --max=N                  raise the --all safety cap (default 10000)

Escape hatch:
  --data @file.json        full search-body override; cannot combine with --all,
                           --page-size, or --max.

Output is JSON on stdout: an array of records (or the raw API response when
--data is used).`

func journalSearch(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("journal search", flag.ContinueOnError)
	flags.SetOutput(cliError)
	from := flags.String("from", "", "filter: postingDate on or after yyyy-mm-dd")
	to := flags.String("to", "", "filter: postingDate on or before yyyy-mm-dd")
	konto := flags.String("konto", "", "filter: accountNumber equals")
	text := flags.String("text", "", "filter: postingText contains substring")
	belegnr := flags.String("belegnr", "", "filter: documentNumber equals")
	amountMin := flags.String("amount-min", "", "filter: amount greater than or equal")
	amountMax := flags.String("amount-max", "", "filter: amount less than or equal")
	dims := repeatedFlag{}
	flags.Var(&dims, "dim", "dimension filter KEY=VALUE; repeatable")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, journalSearchUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("journal search takes no positional arguments")
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
		raw, err := client.JSON(http.MethodPost, "/journal", body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, journalSearchDefaultFields...),
		Order:  []string{"postingDate = asc", "documentNumber = asc"},
	}
	if *konto != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "accountNumber", Operator: scopeskill.OpEquals, Value: *konto,
		})
	}
	if *text != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "postingText", Operator: scopeskill.OpContains, Value: *text,
		})
	}
	if *belegnr != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "documentNumber", Operator: scopeskill.OpEquals, Value: *belegnr,
		})
	}
	if *amountMin != "" {
		amount, err := parseAmountFlag("amount-min", *amountMin)
		if err != nil {
			return err
		}
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "amount", Operator: scopeskill.OpGreaterEq, Value: amount,
		})
	}
	if *amountMax != "" {
		amount, err := parseAmountFlag("amount-max", *amountMax)
		if err != nil {
			return err
		}
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "amount", Operator: scopeskill.OpLessEq, Value: amount,
		})
	}
	for _, dim := range dims {
		cond, err := parseDimensionFilter(dim)
		if err != nil {
			return err
		}
		base.Conditions = append(base.Conditions, cond)
	}

	records, err := paginateJournalSearch(client, base, *from, *to, *all, *pageSize, *max)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}

func paginateJournalSearch(client *scopeskill.Client, base scopeskill.SearchRequest, from, to string, all bool, pageSize, max int) ([]any, error) {
	fetch := func(body map[string]any) ([]any, error) {
		if from != "" {
			fromDate, err := time.Parse(isoDateFormat, from)
			if err != nil {
				return nil, fmt.Errorf("--from: %w", err)
			}
			body["postingDateSince"] = dateMillis(fromDate)
		}
		if to != "" {
			toDate, err := time.Parse(isoDateFormat, to)
			if err != nil {
				return nil, fmt.Errorf("--to: %w", err)
			}
			body["postingDateBefore"] = dateMillis(toDate.AddDate(0, 0, 1))
		}
		raw, err := client.JSON(http.MethodPost, "/journal", body, nil)
		if err != nil {
			return nil, err
		}
		return scopeskill.RecordsFromResponse(raw)
	}
	return scopeskill.Paginate(scopeskill.PaginateOptions{
		All:      all,
		PageSize: pageSize,
		Max:      max,
	}, base, fetch)
}

func parseAmountFlag(name, value string) (float64, error) {
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("--%s must be numeric: %w", name, err)
	}
	return amount, nil
}

func parseDimensionFilter(value string) (scopeskill.SearchCondition, error) {
	key, raw, ok := strings.Cut(value, "=")
	if !ok || key == "" || raw == "" {
		return scopeskill.SearchCondition{}, fmt.Errorf("invalid --dim %q; expected KEY=VALUE", value)
	}
	field, err := dimensionField(key)
	if err != nil {
		return scopeskill.SearchCondition{}, err
	}
	return scopeskill.SearchCondition{
		Field: field, Operator: scopeskill.OpEquals, Value: dimensionValue(raw),
	}, nil
}

func dimensionField(key string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "kostenstelle", "kostenstellen":
		return "documentDimension_1", nil
	case "kostentraeger", "kostenträger":
		return "documentDimension_2", nil
	case "projekt", "projekte":
		return "documentDimension_3", nil
	}
	if strings.HasPrefix(normalized, "dimension_") {
		number := strings.TrimPrefix(normalized, "dimension_")
		if _, err := strconv.Atoi(number); err == nil {
			return "documentDimension_" + number, nil
		}
	}
	if strings.HasPrefix(normalized, "documentdimension_") {
		number := strings.TrimPrefix(normalized, "documentdimension_")
		if _, err := strconv.Atoi(number); err == nil {
			return "documentDimension_" + number, nil
		}
	}
	return "", fmt.Errorf("unknown --dim key %q", key)
}

func dimensionValue(raw string) any {
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return raw
}

func buchungShow(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchung show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli buchung show <documentNumber>")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("buchung show takes exactly one documentNumber")
	}
	documentNumber := flags.Arg(0)

	raw, err := client.JSON(http.MethodGet, "/journal/"+url.PathEscape(documentNumber), nil, nil)
	if err != nil {
		var apiErr scopeskill.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return errors.New(notFoundOrUnauthorisedMessage)
		}
		return err
	}
	lines, err := scopeskill.RecordsFromResponse(raw)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return errors.New(notFoundOrUnauthorisedMessage)
	}

	buchung := map[string]any{
		"documentNumber": documentNumber,
		"lines":          lines,
		"dimensions":     dimensionsFromJournalLines(lines),
	}
	if first, _ := lines[0].(map[string]any); first != nil {
		for _, field := range []string{"postingDate", "documentText", "internalDocumentNumber", "externalDocumentNumber"} {
			if value, ok := first[field]; ok {
				buchung[field] = value
			}
		}
	}

	beleg, err := fetchBelegForBuchung(client, lines)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{
		"buchung": buchung,
		"beleg":   beleg,
	})
}

func dimensionsFromJournalLines(lines []any) map[string]any {
	dimensions := map[string]any{}
	for _, line := range lines {
		rec, ok := line.(map[string]any)
		if !ok {
			continue
		}
		for i := 1; i <= 10; i++ {
			field := fmt.Sprintf("documentDimension_%d", i)
			value, ok := rec[field]
			if !ok || value == nil {
				continue
			}
			dimensions[fmt.Sprintf("dimension_%d", i)] = value
		}
	}
	return dimensions
}

func fetchBelegForBuchung(client *scopeskill.Client, lines []any) (map[string]any, error) {
	var candidates []string
	for _, line := range lines {
		rec, ok := line.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"internalDocumentNumber", "externalDocumentNumber", "documentNumber"} {
			if number := nonEmptyString(rec[field]); number != "" && !containsString(candidates, number) {
				candidates = append(candidates, number)
			}
		}
	}
	for _, number := range candidates {
		for _, endpoint := range []string{
			scopeskill.BelegEndpointIncomingInvoice,
			scopeskill.BelegEndpointOutgoingInvoice,
			scopeskill.BelegEndpointCredit,
		} {
			beleg, err := scopeskill.FetchBeleg(client, endpoint, number)
			if err != nil || beleg != nil {
				return beleg, err
			}
		}
	}
	return nil, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
