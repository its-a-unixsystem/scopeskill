package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type personalAccountKind struct {
	command            string
	outputKey          string
	createEndpoint     string
	searchEndpoint     string
	saldoEndpoint      string
	journalEndpoint    string
	contactNumberField string
}

var (
	debitorAccountKind = personalAccountKind{
		command:            "debitor",
		outputKey:          "debitor",
		createEndpoint:     "/createdebitor",
		searchEndpoint:     "/debitoraccounts",
		saldoEndpoint:      scopeskill.SaldoEndpointDebitor,
		journalEndpoint:    "/openitems/debitor/list",
		contactNumberField: "debitorNumber",
	}
	kreditorAccountKind = personalAccountKind{
		command:            "kreditor",
		outputKey:          "kreditor",
		createEndpoint:     "/createkreditor",
		searchEndpoint:     "/kreditoraccounts",
		saldoEndpoint:      scopeskill.SaldoEndpointKreditor,
		journalEndpoint:    "/openitems/creditor/list",
		contactNumberField: "kreditorNumber",
	}
)

var personalAccountSearchDefaultFields = []string{"number", "name", "active", "contactId"}

var personalAccountKontaktFields = []string{
	"id", "lastname", "companyname", "firstname", "email", "debitorNumber", "kreditorNumber",
}

func debitor(client *scopeskill.Client, args []string) error {
	return personalAccount(client, debitorAccountKind, args)
}

func kreditor(client *scopeskill.Client, args []string) error {
	return personalAccount(client, kreditorAccountKind, args)
}

func personalAccount(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cliOutput, "%s subcommands: search show balance journal bank-connections create\n", kind.command)
		return fmt.Errorf("missing %s subcommand", kind.command)
	}
	switch args[0] {
	case "search":
		return personalAccountSearch(client, kind, args[1:])
	case "show":
		return personalAccountShow(client, kind, args[1:])
	case "balance":
		return personalAccountBalance(client, kind, args[1:])
	case "journal":
		return personalAccountJournal(client, kind, args[1:])
	case "bank-connections":
		return personalAccountBankConnections(client, kind, args[1:])
	case "create":
		return personalAccountCreate(client, kind, args[1:])
	default:
		return fmt.Errorf("unknown %s command: %s", kind.command, args[0])
	}
}

func personalAccountCreateUsage(kind personalAccountKind) string {
	return fmt.Sprintf(`usage: sv-cli %s create --contact-id=N [flags] [--dry-run] [--yes]

Creates a %s for an existing Kontakt via POST %s.

Flags:
  --contact-id=N       required Kontakt Master-ID
  --number=NNNNN       optional personalAccountNumber; omit for automatic assignment
  --number-range=N     optional Nummernkreis number
  --sum-account=NNNN   optional Sammelkonto number
  --data=JSON|@file    full PersonalAccountForm override; mutually exclusive with field flags

Safety:
  --dry-run   validate the Kontakt and preview only; no write
  --yes       bypass the interactive "create %s <contact-id>" confirmation`, kind.command, kind.command, kind.createEndpoint, kind.command)
}

func personalAccountCreate(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" create", flag.ContinueOnError)
	flags.SetOutput(cliError)
	contactIDValue := flags.String("contact-id", "", "Kontakt Master-ID")
	number := flags.String("number", "", "personal account number")
	numberRangeValue := flags.String("number-range", "", "Nummernkreis number")
	sumAccount := flags.String("sum-account", "", "Sammelkonto number")
	data := flags.String("data", "", "PersonalAccountForm JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "validate and preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, personalAccountCreateUsage(kind)) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("%s create takes no positional arguments", kind.command)
	}

	fieldFlagSet := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "contact-id", "number", "number-range", "sum-account":
			fieldFlagSet = true
		}
	})
	if *data != "" && fieldFlagSet {
		return errors.New("--data cannot be combined with individual personal-account field flags")
	}

	var body map[string]any
	if *data != "" {
		var err error
		body, err = loadJSONObject(*data)
		if err != nil {
			return err
		}
	} else {
		if *contactIDValue == "" {
			flags.Usage()
			return fmt.Errorf("%s create requires --contact-id unless --data is used", kind.command)
		}
		body = map[string]any{"contactId": *contactIDValue}
		if *number != "" {
			body["personalAccountNumber"] = *number
		}
		if *numberRangeValue != "" {
			body["numberRangeNumber"] = *numberRangeValue
		}
		if *sumAccount != "" {
			body["sumAccountNumber"] = *sumAccount
		}
	}

	contactID, err := positiveIntegerField(body["contactId"], "contactId")
	if err != nil {
		return err
	}
	body["contactId"] = contactID
	if value, ok := body["numberRangeNumber"]; ok {
		numberRange, err := positiveIntegerField(value, "numberRangeNumber")
		if err != nil {
			return err
		}
		body["numberRangeNumber"] = numberRange
	}

	kontakt, err := scopeskill.FetchKontaktByID(client, contactID)
	if err != nil {
		return err
	}
	if kontakt == nil {
		return errors.New(notFoundOrUnauthorisedMessage)
	}
	if existingNumber := nonEmptyString(kontakt[kind.contactNumberField]); existingNumber != "" {
		account, err := fetchPersonalAccountByNumber(client, kind, existingNumber)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{
			"status":       "already_exists",
			"number":       existingNumber,
			"contactId":    contactID,
			kind.outputKey: account,
			"kontakt":      kontakt,
		})
	}

	req := writeRequest{
		Command:       kind.command + " create",
		Method:        http.MethodPost,
		Path:          kind.createEndpoint,
		Payload:       body,
		ConfirmPhrase: fmt.Sprintf("create %s %d", kind.command, contactID),
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST " + kind.createEndpoint,
			"request":  body,
			"kontakt":  kontakt,
		})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, outcome, writeErr := executeWriteOnce(client, req, func(any) bool { return true })
	if outcome == writeRejected {
		return writeErr
	}
	if outcome == writeAmbiguous {
		_ = printJSON(map[string]any{"status": "verification_required", "response": result})
		if writeErr != nil {
			return fmt.Errorf("%s creation could not be verified: %w", kind.command, writeErr)
		}
		return fmt.Errorf("%s creation could not be verified", kind.command)
	}

	createdKontakt, err := scopeskill.FetchKontaktByID(client, contactID)
	if err != nil {
		return err
	}
	createdNumber := nonEmptyString(createdKontakt[kind.contactNumberField])
	if createdKontakt == nil || createdNumber == "" {
		return fmt.Errorf("created %s could not be read back from Kontakt %d", kind.command, contactID)
	}
	account, err := fetchPersonalAccountByNumber(client, kind, createdNumber)
	if err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("created %s %s could not be read back", kind.command, createdNumber)
	}
	return printJSON(map[string]any{
		"status":       "created",
		"number":       createdNumber,
		"contactId":    contactID,
		kind.outputKey: account,
		"kontakt":      createdKontakt,
	})
}

func positiveIntegerField(value any, field string) (int64, error) {
	text := nonEmptyString(value)
	number, err := strconv.ParseInt(text, 10, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return number, nil
}

func personalAccountSearchUsage(kind personalAccountKind) string {
	return fmt.Sprintf(`usage: sv-cli %s search [filters] [--all] [--max=N] [--page-size=N] [--data @file.json]

Filters:
  --name=SUBSTRING        linked Kontakt lastname contains
  --number=NUMBER         Kontonummer equals
  --number-prefix=PREFIX  Kontonummer starts with
  --active                only active Konten

Pagination:
  default                 single page at pageSize=100
  --page-size=N           override the single-page pageSize (1..1000)
  --all                   page through all results at pageSize=1000, capped at 10000
  --max=N                 raise the --all safety cap (default 10000)

Escape hatch:
  --data @file.json       full search-body override; cannot combine with --all,
                          --page-size, or --max.

Output is JSON on stdout: an array of records (or the raw API response when
--data is used).`, kind.command)
}

func personalAccountSearch(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" search", flag.ContinueOnError)
	flags.SetOutput(cliError)
	name := flags.String("name", "", "filter: linked Kontakt lastname contains substring")
	number := flags.String("number", "", "filter: Kontonummer equals")
	numberPrefix := flags.String("number-prefix", "", "filter: Kontonummer starts with prefix")
	activeOnly := flags.Bool("active", false, "filter: active = true")
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() { fmt.Fprintln(cliError, personalAccountSearchUsage(kind)) }
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
	if *name != "" && *activeOnly {
		return errors.New("--name cannot be combined with --active because linked Kontakt search does not expose account active state")
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

	if *name != "" {
		return personalAccountSearchByKontakt(client, kind, *name, *number, *numberPrefix, *all, *pageSize, *max)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, personalAccountSearchDefaultFields...),
		Order:  []string{"number = asc"},
	}
	if *number != "" {
		base.Conditions = append(base.Conditions, scopeskill.SearchCondition{
			Field: "number", Operator: scopeskill.OpEquals, Value: *number,
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

	records, err := paginatePersonalAccountSearch(client, kind.searchEndpoint, base, *all, *pageSize, *max)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}

func personalAccountSearchByKontakt(client *scopeskill.Client, kind personalAccountKind, name, number, numberPrefix string, all bool, pageSize, max int) error {
	base := scopeskill.SearchRequest{
		Fields: append([]string{}, personalAccountKontaktFields...),
		Order:  []string{"lastname = asc"},
		Conditions: []scopeskill.SearchCondition{
			{Field: "lastname", Operator: scopeskill.OpContains, Value: name},
		},
	}
	contacts, err := paginatePersonalAccountSearch(client, "/contacts", base, all, pageSize, max)
	if err != nil {
		return err
	}
	out := make([]any, 0, len(contacts))
	for _, item := range contacts {
		kontakt, ok := item.(map[string]any)
		if !ok {
			continue
		}
		accountNumber := nonEmptyString(kontakt[kind.contactNumberField])
		if accountNumber == "" {
			continue
		}
		if number != "" && accountNumber != number {
			continue
		}
		if numberPrefix != "" && !strings.HasPrefix(accountNumber, numberPrefix) {
			continue
		}
		out = append(out, map[string]any{
			"number":    accountNumber,
			"name":      kontaktDisplayName(kontakt),
			"contactId": kontakt["id"],
			"kontakt":   kontakt,
		})
	}
	return printJSON(out)
}

func paginatePersonalAccountSearch(client *scopeskill.Client, endpoint string, base scopeskill.SearchRequest, all bool, pageSize, max int) ([]any, error) {
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

func kontaktDisplayName(kontakt map[string]any) string {
	for _, field := range []string{"lastname", "companyname", "firstname"} {
		if v := nonEmptyString(kontakt[field]); v != "" {
			return v
		}
	}
	return ""
}

func fetchPersonalAccountByNumber(client *scopeskill.Client, kind personalAccountKind, number string) (map[string]any, error) {
	req := scopeskill.SearchRequest{
		PageSize: 1,
		Fields:   append([]string{}, personalAccountSearchDefaultFields...),
		Conditions: []scopeskill.SearchCondition{
			{Field: "number", Operator: scopeskill.OpEquals, Value: number},
		},
	}
	body, err := req.Body()
	if err != nil {
		return nil, err
	}
	raw, err := client.JSON(http.MethodPost, kind.searchEndpoint, body, nil)
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

func personalAccountShow(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s show <Kontonummer>\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s show takes exactly one Kontonummer", kind.command)
	}
	number := flags.Arg(0)

	account, err := fetchPersonalAccountByNumber(client, kind, number)
	if err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("%s %s not found", kind.command, number)
	}

	var kontakt any
	if contactID := account["contactId"]; contactID != nil {
		k, err := scopeskill.FetchKontaktByID(client, contactID)
		if err != nil {
			return err
		}
		kontakt = k
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

	saldoCurrent, err := scopeskill.FetchSaldo(client, kind.saldoEndpoint, number, earliest.Beginning, now)
	if err != nil {
		return err
	}
	saldoFYtD, err := scopeskill.FetchSaldo(client, kind.saldoEndpoint, number, current.Beginning, now)
	if err != nil {
		return err
	}

	return printJSON(map[string]any{
		kind.outputKey: account,
		"kontakt":      kontakt,
		"saldo": map[string]any{
			"current":          saldoCurrent,
			"fiscalYearToDate": saldoFYtD,
		},
	})
}

func personalAccountBalance(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" balance", flag.ContinueOnError)
	flags.SetOutput(cliError)
	from := flags.String("from", "", "start date (yyyy-mm-dd); default = current fiscal year start")
	to := flags.String("to", "", "end date (yyyy-mm-dd); default = today")
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s balance <Kontonummer> [--from=YYYY-MM-DD] [--to=YYYY-MM-DD]\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s balance takes exactly one Kontonummer", kind.command)
	}
	number := flags.Arg(0)

	now := nowFunc()
	fromDate, toDate, err := resolveSaldoRange(client, *from, *to, now)
	if err != nil {
		return err
	}
	rec, err := fetchSaldoForBalance(
		client,
		kind.saldoEndpoint,
		kind.command,
		number,
		fromDate,
		toDate,
		func(client *scopeskill.Client, number string) (map[string]any, error) {
			return fetchPersonalAccountByNumber(client, kind, number)
		},
	)
	if err != nil {
		return err
	}
	return printJSON(rec)
}

func personalAccountJournal(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	return runSearchEndpointCommand(client, searchEndpointCommand{
		name:     kind.command + " journal",
		endpoint: kind.journalEndpoint,
	}, args)
}

func personalAccountBankConnections(client *scopeskill.Client, kind personalAccountKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" bank-connections", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s bank-connections <Kontonummer>\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s bank-connections takes exactly one Kontonummer", kind.command)
	}
	number := flags.Arg(0)
	raw, err := client.JSON(http.MethodGet, kind.searchEndpoint+"/"+url.PathEscape(number)+"/bankConnections", nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
