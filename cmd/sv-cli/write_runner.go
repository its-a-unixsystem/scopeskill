package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

// writeRequest describes one mutating API call. writePreview, confirmWrite,
// and executeWriteOnce are the shared runner for all future write commands.
type writeRequest struct {
	Command       string
	Method        string
	Path          string
	Payload       any
	ConfirmPhrase string
}

type writeOptions struct {
	Yes bool
}

type writeOutcome int

const (
	writeAccepted writeOutcome = iota
	writeRejected
	writeAmbiguous
)

// writePreview prints the endpoint and canonical payload to stderr; tokens are
// never part of the output.
func writePreview(client *scopeskill.Client, req writeRequest) {
	fmt.Fprintf(cliError, "sv-cli %s preview\n", req.Command)
	fmt.Fprintf(cliError, "customer: %s\n", client.Config.Customer)
	fmt.Fprintf(cliError, "base URL: %s\n", client.Config.BaseURL)
	fmt.Fprintf(cliError, "SKR: %s\n", client.Config.SKR)
	fmt.Fprintf(cliError, "endpoint: %s %s\n", req.Method, req.Path)
	raw, _ := json.MarshalIndent(req.Payload, "", "  ")
	fmt.Fprintln(cliError, string(raw))
}

// confirmWrite gates the write on --yes or an interactive confirmation.
func confirmWrite(req writeRequest, opts writeOptions) error {
	if opts.Yes {
		return nil
	}
	if !isTerminal(cliInput) {
		return fmt.Errorf("sv-cli %s requires a TTY or --yes; stdin is not interactive", req.Command)
	}
	answer, err := promptLine(fmt.Sprintf("Type %q to continue: ", req.ConfirmPhrase))
	if err != nil {
		return err
	}
	if answer != req.ConfirmPhrase {
		return errors.New("confirmation did not match; aborted")
	}
	return nil
}

// executeWriteOnce issues exactly one request and classifies the result:
// 4xx is a rejection (the write did not happen), anything else non-OK is
// ambiguous (the write may have happened). There is no retry.
func executeWriteOnce(client *scopeskill.Client, req writeRequest, accept func(result any) bool) (any, writeOutcome, error) {
	result, err := client.JSON(req.Method, req.Path, req.Payload, nil)
	if err != nil {
		var apiErr scopeskill.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= http.StatusBadRequest && apiErr.StatusCode < http.StatusInternalServerError {
			return nil, writeRejected, err
		}
		return nil, writeAmbiguous, err
	}
	if accept(result) {
		return result, writeAccepted, nil
	}
	return result, writeAmbiguous, nil
}
