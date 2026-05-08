package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type searchEndpointCommand struct {
	name          string
	endpoint      string
	defaultFields []string
	order         []string
}

func runSearchEndpointCommand(client *scopeskill.Client, command searchEndpointCommand, args []string) error {
	flags := flag.NewFlagSet(command.name, flag.ContinueOnError)
	flags.SetOutput(cliError)
	data := flags.String("data", "", "JSON body, or @path/to/file.json (full override)")
	all := flags.Bool("all", false, "page through all results")
	pageSize := flags.Int("page-size", 0, "page size for the single-page request (default 100)")
	max := flags.Int("max", 0, "result cap when --all is set (default 10000)")
	flags.Usage = func() {
		fmt.Fprintf(cliError, "usage: sv-cli %s [--all] [--max=N] [--page-size=N] [--data @file.json]\n", command.name)
	}
	if err := flags.Parse(normalizeFlagArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return fmt.Errorf("%s takes no positional arguments", command.name)
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
		raw, err := client.JSON(http.MethodPost, command.endpoint, body, nil)
		if err != nil {
			return err
		}
		return printJSON(raw)
	}

	base := scopeskill.SearchRequest{
		Fields: append([]string{}, command.defaultFields...),
		Order:  append([]string{}, command.order...),
	}
	records, err := paginateSearchEndpoint(client, command.endpoint, base, *all, *pageSize, *max)
	if err != nil {
		return err
	}
	if records == nil {
		records = []any{}
	}
	return printJSON(records)
}

func paginateSearchEndpoint(client *scopeskill.Client, endpoint string, base scopeskill.SearchRequest, all bool, pageSize, max int) ([]any, error) {
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
