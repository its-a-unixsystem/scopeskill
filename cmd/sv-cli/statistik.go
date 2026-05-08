package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var (
	statistikKontoSearchCommand = searchEndpointCommand{
		name:          "statistik konto search",
		endpoint:      "/statisticsaccounts",
		defaultFields: []string{"id", "number", "name"},
		order:         []string{"number = asc"},
	}
	statistikBuchungSearchCommand = searchEndpointCommand{
		name:          "statistik buchung search",
		endpoint:      "/statisticspostings",
		defaultFields: []string{"rowNumber", "postingDate", "accountNumber", "amount", "postingText"},
		order:         []string{"rowNumber = asc"},
	}
)

func statistik(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "statistik subcommands: konto buchung")
		return errors.New("missing statistik subcommand")
	}
	switch args[0] {
	case "konto":
		return statistikKonto(client, args[1:])
	case "buchung":
		return statistikBuchung(client, args[1:])
	default:
		return fmt.Errorf("unknown statistik command: %s", args[0])
	}
}

func statistikKonto(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "statistik konto subcommands: search show")
		return errors.New("missing statistik konto subcommand")
	}
	switch args[0] {
	case "search":
		return runSearchEndpointCommand(client, statistikKontoSearchCommand, args[1:])
	case "show":
		return statistikShow(client, "statistik konto show", "/statisticsaccount", "number", args[1:])
	default:
		return fmt.Errorf("unknown statistik konto command: %s", args[0])
	}
}

func statistikBuchung(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "statistik buchung subcommands: search show")
		return errors.New("missing statistik buchung subcommand")
	}
	switch args[0] {
	case "search":
		return runSearchEndpointCommand(client, statistikBuchungSearchCommand, args[1:])
	case "show":
		return statistikShow(client, "statistik buchung show", "/statisticspostings", "rowNumber", args[1:])
	default:
		return fmt.Errorf("unknown statistik buchung command: %s", args[0])
	}
}

func statistikShow(client *scopeskill.Client, command, endpoint, argName string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s <%s>\n", command, argName)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s takes exactly one %s", command, argName)
	}
	raw, err := client.JSON(http.MethodGet, endpoint+"/"+url.PathEscape(flags.Arg(0)), nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
