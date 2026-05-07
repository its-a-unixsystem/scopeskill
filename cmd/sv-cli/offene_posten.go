package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type offenePostenSeite struct {
	name          string
	endpoint      string
	accountKind   personalAccountKind
	belegEndpoint string
}

var (
	offenePostenDebitor = offenePostenSeite{
		name:          "debitor",
		endpoint:      "/openitems/debtors",
		accountKind:   debitorAccountKind,
		belegEndpoint: scopeskill.BelegEndpointOutgoingInvoice,
	}
	offenePostenKreditor = offenePostenSeite{
		name:          "kreditor",
		endpoint:      "/openitems/creditors",
		accountKind:   kreditorAccountKind,
		belegEndpoint: scopeskill.BelegEndpointIncomingInvoice,
	}
)

func offenePosten(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "offene-posten subcommands: list show")
		return errors.New("missing offene-posten subcommand")
	}
	switch args[0] {
	case "list":
		return offenePostenList(client, args[1:])
	case "show":
		return offenePostenShow(client, args[1:])
	default:
		return fmt.Errorf("unknown offene-posten command: %s", args[0])
	}
}

const offenePostenListUsage = `usage: sv-cli offene-posten list --seite=debitor|kreditor [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters:
  --seite=debitor|kreditor  required OP side
  --overdue                 dueDate less than today
  --due-before=YYYY-MM-DD   dueDate less than date
  --kontakt-id=ID           linked Kontakt id equals
  --konto=NUMBER            OP accountNumber equals

Pagination:
  default                   single page at pageSize=100
  --page-size=N             override the single-page pageSize (1..1000)
  --all                     page through all results at pageSize=1000, capped at 10000
  --max=N                   raise the --all safety cap (default 10000)

Escape hatch:
  --data @file.json         full search-body override; cannot combine with --all,
                            --page-size, or --max.

Output is JSON on stdout: an array of records (or the raw API response when
--data is used).`

func offenePostenList(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("offene-posten list", flag.ContinueOnError)
	flags.SetOutput(cliError)
	seiteFlag := flags.String("seite", "", "required OP side: debitor or kreditor")
	overdue := flags.Bool("overdue", false, "filter: dueDate less than today")
	dueBefore := flags.String("due-before", "", "filter: dueDate less than yyyy-mm-dd")
	kontaktID := flags.String("kontakt-id", "", "filter: linked Kontakt id equals")
	konto := flags.String("konto", "", "filter: accountNumber equals")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, offenePostenListUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("offene-posten list takes no positional arguments")
	}
	seite, err := parseOffenePostenSeite(*seiteFlag)
	if err != nil {
		return err
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
		raw, err := client.JSON(http.MethodPost, seite.endpoint, body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	accountNumber := *konto
	if *kontaktID != "" {
		kontakt, err := scopeskill.FetchKontaktByID(client, *kontaktID)
		if err != nil {
			return err
		}
		linked := ""
		if kontakt != nil {
			linked = nonEmptyString(kontakt[seite.accountKind.contactNumberField])
		}
		if linked == "" || (accountNumber != "" && accountNumber != linked) {
			return printJSON([]any{})
		}
		accountNumber = linked
	}

	base := scopeskill.SearchRequest{}
	if *overdue {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "dueDate", Operator: scopeskill.OpLess, Value: dateMillis(today(nowFunc())),
		})
	}
	if *dueBefore != "" {
		d, err := time.Parse(isoDateFormat, *dueBefore)
		if err != nil {
			return fmt.Errorf("--due-before: %w", err)
		}
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "dueDate", Operator: scopeskill.OpLess, Value: dateMillis(d),
		})
	}
	if accountNumber != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "accountNumber", Operator: scopeskill.OpEquals, Value: accountNumber,
		})
	}

	records, err := paginateOpenItems(client, seite.endpoint, base, *all, *pageSize, *max)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}

func offenePostenShow(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("offene-posten show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli offene-posten show <id>")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("offene-posten show takes exactly one id")
	}
	id, err := strconv.Atoi(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("offene-posten show id must be numeric: %w", err)
	}

	op, seite, err := fetchOpenItemByID(client, id)
	if err != nil {
		return err
	}
	if op == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}

	beleg, err := fetchBelegForOpenItem(client, seite, op)
	if err != nil {
		return err
	}
	kontakt, err := fetchKontaktForOpenItem(client, seite, op)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		"op":      op,
		"beleg":   beleg,
		"kontakt": kontakt,
	})
}

func parseOffenePostenSeite(value string) (offenePostenSeite, error) {
	switch value {
	case "debitor":
		return offenePostenDebitor, nil
	case "kreditor":
		return offenePostenKreditor, nil
	case "":
		return offenePostenSeite{}, errors.New("missing required --seite=debitor|kreditor")
	default:
		return offenePostenSeite{}, fmt.Errorf("--seite must be debitor or kreditor, got %q", value)
	}
}

func paginateOpenItems(client *scopeskill.Client, endpoint string, base scopeskill.SearchRequest, all bool, pageSize, max int) ([]any, error) {
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

func fetchOpenItemByID(client *scopeskill.Client, id int) (map[string]any, offenePostenSeite, error) {
	for _, seite := range []offenePostenSeite{offenePostenDebitor, offenePostenKreditor} {
		req := scopeskill.SearchRequest{
			PageSize: 1,
			Conditions: []scopeskill.SearchCondition{
				{Field: "leadRowNumber", Operator: scopeskill.OpEquals, Value: id},
			},
		}
		body, err := req.Body()
		if err != nil {
			return nil, offenePostenSeite{}, err
		}
		raw, err := client.JSON(http.MethodPost, seite.endpoint, body, nil)
		if err != nil {
			return nil, offenePostenSeite{}, err
		}
		records, err := scopeskill.RecordsFromResponse(raw)
		if err != nil {
			return nil, offenePostenSeite{}, err
		}
		if len(records) == 0 {
			continue
		}
		rec, _ := records[0].(map[string]any)
		return rec, seite, nil
	}
	return nil, offenePostenSeite{}, nil
}

func fetchBelegForOpenItem(client *scopeskill.Client, seite offenePostenSeite, op map[string]any) (map[string]any, error) {
	number := nonEmptyString(op["invoiceNumber"])
	if number == "" {
		return nil, nil
	}
	beleg, err := scopeskill.FetchBeleg(client, seite.belegEndpoint, number)
	if err != nil || beleg != nil || seite.name != "debitor" {
		return beleg, err
	}
	return scopeskill.FetchBeleg(client, scopeskill.BelegEndpointCredit, number)
}

func fetchKontaktForOpenItem(client *scopeskill.Client, seite offenePostenSeite, op map[string]any) (map[string]any, error) {
	accountNumber := nonEmptyString(op["accountNumber"])
	if accountNumber == "" {
		return nil, nil
	}
	account, err := fetchPersonalAccountByNumber(client, seite.accountKind, accountNumber)
	if err != nil || account == nil {
		return nil, err
	}
	contactID := account["contactId"]
	if contactID == nil {
		return nil, nil
	}
	return scopeskill.FetchKontaktByID(client, contactID)
}

func today(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func dateMillis(t time.Time) int64 {
	return today(t).Unix() * 1000
}
