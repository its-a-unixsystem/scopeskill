package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type configCommandKind struct {
	command      string
	listEndpoint string
	showEndpoint string
}

var (
	zahlungsbedingungKind = configCommandKind{
		command:      "zahlungsbedingung",
		listEndpoint: "/paymentterms",
		showEndpoint: "/paymentterm",
	}
	steuermatrixKind = configCommandKind{
		command:      "steuermatrix",
		listEndpoint: "/vatmatrixentries",
	}
	steuersachverhaltKind = configCommandKind{
		command:      "steuersachverhalt",
		listEndpoint: "/vatscopes",
	}
)

func zahlungsbedingung(client *scopeskill.Client, args []string) error {
	return configCommand(client, zahlungsbedingungKind, args)
}

func steuermatrix(client *scopeskill.Client, args []string) error {
	return configCommand(client, steuermatrixKind, args)
}

func steuersachverhalt(client *scopeskill.Client, args []string) error {
	return configCommand(client, steuersachverhaltKind, args)
}

func configCommand(client *scopeskill.Client, kind configCommandKind, args []string) error {
	if len(args) == 0 {
		fmt.Fprintf(cliOutput, "%s subcommands: list", kind.command)
		if kind.showEndpoint != "" {
			fmt.Fprint(cliOutput, " show")
		}
		fmt.Fprintln(cliOutput)
		return fmt.Errorf("missing %s subcommand", kind.command)
	}
	switch args[0] {
	case "list":
		return configCommandList(client, kind, args[1:])
	case "show":
		if kind.showEndpoint == "" {
			return fmt.Errorf("unknown %s command: show", kind.command)
		}
		return configCommandShow(client, kind, args[1:])
	default:
		return fmt.Errorf("unknown %s command: %s", kind.command, args[0])
	}
}

func configCommandList(client *scopeskill.Client, kind configCommandKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" list", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s list\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("%s list takes no positional arguments", kind.command)
	}
	raw, err := client.JSON(http.MethodGet, kind.listEndpoint, nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}

func configCommandShow(client *scopeskill.Client, kind configCommandKind, args []string) error {
	flags := flag.NewFlagSet(kind.command+" show", flag.ContinueOnError)
	flags.SetOutput(cliError)
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s show <id>\n", kind.command)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("%s show takes exactly one id", kind.command)
	}
	raw, err := client.JSON(http.MethodGet, kind.showEndpoint+"/"+url.PathEscape(flags.Arg(0)), nil, nil)
	if err != nil {
		return err
	}
	return printJSON(raw)
}
