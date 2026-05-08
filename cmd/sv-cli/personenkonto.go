package main

import (
	"errors"
	"fmt"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

var personenkontoJournalCommand = searchEndpointCommand{
	name:     "personenkonto journal",
	endpoint: "/personaljournal",
}

func personenkonto(client *scopeskill.Client, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cliOutput, "personenkonto subcommands: journal")
		return errors.New("missing personenkonto subcommand")
	}
	switch args[0] {
	case "journal":
		return runSearchEndpointCommand(client, personenkontoJournalCommand, args[1:])
	default:
		return fmt.Errorf("unknown personenkonto command: %s", args[0])
	}
}
