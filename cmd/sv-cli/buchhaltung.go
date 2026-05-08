package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

func buchhaltung(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "buchhaltung subcommands: info mapping gewinn-und-verlust")
		return errors.New("missing buchhaltung subcommand")
	}
	switch args[0] {
	case "info":
		return buchhaltungGet(client, "buchhaltung info", "/accountinginfo", args[1:])
	case "mapping":
		return buchhaltungGet(client, "buchhaltung mapping", "/accountmapping", args[1:])
	case "gewinn-und-verlust":
		return buchhaltungGewinnUndVerlust(client, args[1:])
	default:
		return fmt.Errorf("unknown buchhaltung command: %s", args[0])
	}
}

func buchhaltungGet(client *scopeskill.Client, command, endpoint string, args []string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s\n", command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("%s takes no positional arguments", command)
	}
	raw, err := client.JSON(http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}

func buchhaltungGewinnUndVerlust(client *scopeskill.Client, args []string) error {
	flags := flag.NewFlagSet("buchhaltung gewinn-und-verlust", flag.ContinueOnError)
	flags.SetOutput(cliError)
	balanceDate := flags.String("balance-date", "", "optional balance date in dd.MM.yyyy")
	flags.Usage = func() {
		fmt.Fprintln(cliError, "usage: sv-cli buchhaltung gewinn-und-verlust [--balance-date=DD.MM.YYYY]")
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return errors.New("buchhaltung gewinn-und-verlust takes no positional arguments")
	}
	query := map[string]string{}
	if *balanceDate != "" {
		query["balanceDate"] = *balanceDate
	}
	raw, err := client.JSON(http.MethodGet, "/gainandlossadjustmentaccounts", nil, query)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
