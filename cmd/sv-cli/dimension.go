package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var dimensionSearchCommand = searchEndpointCommand{
	name:     "dimension search",
	endpoint: "/dimensions",
}

func dimension(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "dimension subcommands: search entries")
		return errors.New("missing dimension subcommand")
	}
	switch args[0] {
	case "search":
		return runSearchEndpointCommand(client, dimensionSearchCommand, args[1:])
	case "entries":
		return dimensionEntries(client, args[1:])
	default:
		return fmt.Errorf("unknown dimension command: %s", args[0])
	}
}

func dimensionEntries(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("dimension entries", flag.ContinueOnError)
	flags.SetOutput(cliError)
	page := flags.Int("page", -1, "page number")
	pageSize := flags.Int("page-size", 0, "page size")
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli dimension entries <dimension> [--page=N] [--page-size=N]")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("dimension entries takes exactly one dimension")
	}
	if *page < -1 {
		return errors.New("--page must be non-negative")
	}
	if *pageSize < 0 || *pageSize > scopeskill.MaxSearchPageSize {
		return fmt.Errorf("--page-size must be between 1 and %d", scopeskill.MaxSearchPageSize)
	}
	query := map[string]string{}
	if *page >= 0 {
		query["page"] = fmt.Sprint(*page)
	}
	if *pageSize > 0 {
		query["pageSize"] = fmt.Sprint(*pageSize)
	}
	raw, err := client.JSON(http.MethodGet, "/dimensions/"+url.PathEscape(flags.Arg(0))+"/dimensionentries", nil, query)
	if err != nil {
		return err
	}
	return printJSON(raw)
}

func textbaustein(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "textbaustein subcommands: list")
		return errors.New("missing textbaustein subcommand")
	}
	switch args[0] {
	case "list":
		return textbausteinList(client, args[1:])
	default:
		return fmt.Errorf("unknown textbaustein command: %s", args[0])
	}
}

func textbausteinList(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("textbaustein list", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli textbaustein list")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("textbaustein list takes no positional arguments")
	}
	raw, err := client.JSON(http.MethodGet, "/texttemplates", nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
