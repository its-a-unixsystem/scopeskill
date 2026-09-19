package main

import (
	"errors"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

func buchungCorrect(_ *scopeskill.Client, _ []string) error {
	return correctionUnavailable("buchung correct", "POST /correctpostings")
}

func buchungCorrectImport(_ *scopeskill.Client, _ []string) error {
	return correctionUnavailable("buchung correct-import", "POST /postings/correction")
}

func correctionUnavailable(command, endpoint string) error {
	reason := command + " is unavailable: " + endpoint + " correction semantics and request contract have not been proven by the required controlled live contract test; use separately confirmed 'buchung cancel' and 'buchung create' commands"
	_ = printJSON(map[string]any{
		"status":   "conflict",
		"state":    "conflict",
		"command":  command,
		"endpoint": endpoint,
		"reason":   reason,
	})
	return errors.New(reason)
}
