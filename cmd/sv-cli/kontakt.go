package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var kontaktSearchDefaultFields = []string{
	"id", "lastname", "companyname", "firstname",
	"email", "vatId",
	"debitorNumber", "kreditorNumber",
}

func kontakt(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "kontakt subcommands: search show create")
		return errors.New("missing kontakt subcommand")
	}
	switch args[0] {
	case "search":
		return kontaktSearch(client, args[1:])
	case "show":
		return kontaktShow(client, args[1:])
	case "create":
		return kontaktCreate(client, args[1:])
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

const kontaktCreateUsage = `usage: sv-cli kontakt create --name=STRING [flags] [--dry-run] [--yes]

Creates a Kontakt via POST /contact/new.

Flags:
  --name=STRING             required; lastname / company name
  --type=company|person     default company
  --firstname=STRING        person first name
  --salutation=STRING       person salutation
  --vat-id=STRING           Umsatzsteuer-ID
  --email=STRING            business email
  --street=STRING           main-address street
  --city=STRING             main-address city
  --postcode=STRING         main-address postcode
  --country=STRING          main-address country
  --customer-number=STRING  optional manual Kontaktnummer
  --data=JSON|@file         full KontaktForm override; mutually exclusive with field flags

Safety:
  --dry-run   preview only; no write
  --yes       bypass the interactive "create kontakt <name>" confirmation`

func kontaktCreate(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("kontakt create", flag.ContinueOnError)
	flags.SetOutput(cliError)
	name := flags.String("name", "", "lastname / company name")
	contactType := flags.String("type", "company", "company or person")
	firstname := flags.String("firstname", "", "person first name")
	salutation := flags.String("salutation", "", "person salutation")
	vatID := flags.String("vat-id", "", "Umsatzsteuer-ID")
	email := flags.String("email", "", "business email")
	street := flags.String("street", "", "main-address street")
	city := flags.String("city", "", "main-address city")
	postcode := flags.String("postcode", "", "main-address postcode")
	country := flags.String("country", "", "main-address country")
	customerNumber := flags.String("customer-number", "", "manual Kontaktnummer")
	data := flags.String("data", "", "KontaktForm JSON, or @path/to/file.json")
	dryRun := flags.Bool("dry-run", false, "preview only; no write")
	yes := flags.Bool("yes", false, "skip the interactive confirmation")
	flags.Usage = func() { fmt.Fprintln(cliError, kontaktCreateUsage) }
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("kontakt create takes no positional arguments")
	}

	fieldFlagSet := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "name", "type", "firstname", "salutation", "vat-id", "email", "street", "city", "postcode", "country", "customer-number":
			fieldFlagSet = true
		}
	})
	if *data != "" && fieldFlagSet {
		return errors.New("--data cannot be combined with individual Kontakt field flags")
	}

	var body map[string]any
	if *data != "" {
		var err error
		body, err = loadJSONObject(*data)
		if err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(*name) == "" {
			flags.Usage()
			return errors.New("kontakt create requires --name unless --data is used")
		}
		if *contactType != "company" && *contactType != "person" {
			return errors.New("--type must be company or person")
		}
		if *contactType == "company" && (*firstname != "" || *salutation != "") {
			return errors.New("--firstname and --salutation require --type=person")
		}
		body = map[string]any{
			"lastname": *name,
			"person":   *contactType == "person",
		}
		for key, value := range map[string]string{
			"firstname":      *firstname,
			"salutation":     *salutation,
			"vatId":          *vatID,
			"email":          *email,
			"street1":        *street,
			"city1":          *city,
			"postcode1":      *postcode,
			"country1":       *country,
			"customerNumber": *customerNumber,
		} {
			if value != "" {
				body[key] = value
			}
		}
	}
	contactName, _ := body["lastname"].(string)
	if strings.TrimSpace(contactName) == "" {
		return errors.New("KontaktForm lastname must be a non-empty string")
	}

	req := writeRequest{
		Command:       "kontakt create",
		Method:        http.MethodPost,
		Path:          "/contact/new",
		Payload:       body,
		ConfirmPhrase: "create kontakt " + contactName,
	}
	writePreview(client, req)
	if *dryRun {
		return printJSON(map[string]any{
			"status":   "dry_run",
			"endpoint": "POST /contact/new",
			"request":  body,
		})
	}
	if err := confirmWrite(req, writeOptions{Yes: *yes}); err != nil {
		return err
	}
	result, outcome, writeErr := executeWriteOnce(client, req, func(result any) bool {
		obj, ok := result.(map[string]any)
		return ok && nonEmptyString(obj["contactId"]) != ""
	})
	if outcome == writeRejected {
		return writeErr
	}
	if outcome != writeAccepted {
		_ = printJSON(map[string]any{"status": "verification_required", "response": result})
		if writeErr != nil {
			return fmt.Errorf("Kontakt creation could not be verified: %w", writeErr)
		}
		return errors.New("Kontakt creation response did not contain contactId")
	}
	response := result.(map[string]any)
	contactID := response["contactId"]
	created, err := scopeskill.FetchKontaktByID(client, contactID)
	if err != nil {
		return err
	}
	if created == nil {
		return errors.New("created Kontakt could not be read back")
	}
	return printJSON(map[string]any{
		"status":    "created",
		"contactId": contactID,
		"kontakt":   created,
	})
}

func nonEmptyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%g", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}
