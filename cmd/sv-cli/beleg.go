package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type belegKind struct {
	command        string
	outputKey      string
	searchEndpoint string
	showEndpoint   string
}

var (
	eingangsrechnungKind = belegKind{
		command:        "eingangsrechnung",
		outputKey:      "eingangsrechnung",
		searchEndpoint: "/incominginvoices",
		showEndpoint:   scopeskill.BelegEndpointIncomingInvoice,
	}
	gutschriftKind = belegKind{
		command:        "gutschrift",
		outputKey:      "gutschrift",
		searchEndpoint: "/credits",
		showEndpoint:   scopeskill.BelegEndpointCredit,
	}
)

var belegSearchDefaultFields = []string{
	"documentNumber",
	"number",
	"vendorName",
	"vendorContactId",
	"vendorPersonalAccountId",
	"contentStateId",
	"paymentStateId",
	"postingStateId",
}

var belegKreditorAccountFields = []string{"id", "number", "name", "contactId"}

func eingangsrechnung(client *scopeskill.Client, args []string) error {
	return beleg(client, eingangsrechnungKind, args)
}

func gutschrift(client *scopeskill.Client, args []string) error {
	return beleg(client, gutschriftKind, args)
}

func beleg(client *scopeskill.Client, kind belegKind, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cliOutput, "%s subcommands: search show\n", kind.command)
		return fmt.Errorf("missing %s subcommand", kind.command)
	}
	switch args[0] {
	case "search":
		return belegSearch(client, kind, args[1:])
	case "show":
		return belegShow(client, kind, args[1:])
	default:
		return fmt.Errorf("unknown %s command: %s", kind.command, args[0])
	}
}

func belegSearchUsage(kind belegKind) string {
	return fmt.Sprintf(`usage: sv-cli %s search [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters:
  --document-number=VALUE  Belegnummer equals
  --vendor-name=SUBSTRING  vendor name contains
  --content-state=ID       contentStateId equals
  --payment-state=ID       paymentStateId equals
  --posting-state=ID       postingStateId equals

Pagination:
  default                  single page at pageSize=100
  --page-size=N            override the single-page pageSize (1..1000)
  --all                    page through all results at pageSize=1000, capped at 10000
  --max=N                  raise the --all safety cap (default 10000)

Escape hatch:
  --data @file.json        full search-body override; cannot combine with --all,
                           --page-size, or --max.

Output is JSON on stdout: an array of records (or the raw API response when
--data is used).`, kind.command)
}

func belegSearch(client *scopeskill.Client, kind belegKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" search", flag.ContinueOnError)
	flags.SetOutput(cliError)
	documentNumber := flags.String("document-number", "", "filter: Belegnummer equals")
	vendorName := flags.String("vendor-name", "", "filter: vendor name contains substring")
	contentState := flags.String("content-state", "", "filter: contentStateId equals")
	paymentState := flags.String("payment-state", "", "filter: paymentStateId equals")
	postingState := flags.String("posting-state", "", "filter: postingStateId equals")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, belegSearchUsage(kind)) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("%s search takes no positional arguments", kind.command)
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
		raw, err := client.JSON(http.MethodPost, kind.searchEndpoint, body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, belegSearchDefaultFields...),
		Order:  []string{"documentNumber = asc"},
	}
	if *documentNumber != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "documentNumber", Operator: scopeskill.OpEquals, Value: *documentNumber,
		})
	}
	if *vendorName != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "vendorName", Operator: scopeskill.OpContains, Value: *vendorName,
		})
	}
	if *contentState != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "contentStateId", Operator: scopeskill.OpEquals, Value: *contentState,
		})
	}
	if *paymentState != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "paymentStateId", Operator: scopeskill.OpEquals, Value: *paymentState,
		})
	}
	if *postingState != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "postingStateId", Operator: scopeskill.OpEquals, Value: *postingState,
		})
	}

	records, err := paginateBelegSearch(client, kind.searchEndpoint, base, *all, *pageSize, *max)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}

func paginateBelegSearch(client *scopeskill.Client, endpoint string, base scopeskill.SearchRequest, all bool, pageSize, max int) ([]any, error) {
	fetch := func(body map[string]any) ([]any, error) {
		raw, err := client.JSON(http.MethodPost, endpoint, body, nil)
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

func belegShow(client *scopeskill.Client, kind belegKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s show <Belegnummer>\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s show takes exactly one Belegnummer", kind.command)
	}
	number := flags.Arg(0)

	beleg, err := scopeskill.FetchBeleg(client, kind.showEndpoint, number)
	if err != nil {
		return err
	}
	if beleg == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}
	kontakt, err := fetchKontaktForBeleg(client, beleg)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		kind.outputKey: beleg,
		"kontakt":      kontakt,
	})
}

func fetchKontaktForBeleg(client *scopeskill.Client, beleg map[string]any) (map[string]any, error) {
	if contactID := beleg["vendorContactId"]; nonEmptyString(contactID) != "" {
		return scopeskill.FetchKontaktByID(client, contactID)
	}
	accountID := beleg["vendorPersonalAccountId"]
	if nonEmptyString(accountID) == "" {
		return nil, nil
	}
	account, err := fetchKreditorAccountByID(client, accountID)
	if err != nil || account == nil {
		return nil, err
	}
	contactID := account["contactId"]
	if nonEmptyString(contactID) == "" {
		return nil, nil
	}
	return scopeskill.FetchKontaktByID(client, contactID)
}

func fetchKreditorAccountByID(client *scopeskill.Client, id any) (map[string]any, error) {
	req := scopeskill.SearchRequest{
		PageSize: 1,
		Fields:   append([]string{}, belegKreditorAccountFields...),
		Conditions: []scopeskill.SearchCondition{
			{Field: "id", Operator: scopeskill.OpEquals, Value: id},
		},
	}
	body, err := req.Body()
	if err != nil {
		return nil, err
	}
	raw, err := client.JSON(http.MethodPost, kreditorAccountKind.searchEndpoint, body, nil)
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
