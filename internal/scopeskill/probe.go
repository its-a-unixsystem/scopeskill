package scopeskill

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	ConfigKeySKR = "SKR"
	SKR03        = "skr03"
	SKR04        = "skr04"
)

// MissingSKRMessage is the user-facing error message accounting commands emit
// when SKR is not configured. Centralised so the wording stays consistent.
const MissingSKRMessage = "SKR not configured; run `auth login` again or set `SKR=skr03|skr04` in your scopeskill config"

// RequireSKR is the shared helper accounting commands call before doing
// anything that depends on the configured chart of accounts.
func RequireSKR(values map[string]string) error {
	if values[ConfigKeySKR] != "" {
		return nil
	}
	return errors.New(MissingSKRMessage)
}

// ProbeContext carries everything an Unternehmen probe may need: an
// authenticated Client, the ConfigFile to write into, a writer for status
// messages, and a TTY prompt callback for the fallback path. Per-probe
// non-interactive overrides (currently only SKRFlag) live here too.
type ProbeContext struct {
	Client  *Client
	Config  *ConfigFile
	Stderr  io.Writer
	Prompt  func(message string) (string, error)
	SKRFlag string
}

// Probe runs one Unternehmen probe and writes zero or more keys into the
// supplied ConfigFile.
type Probe func(ctx *ProbeContext) error

// DefaultProbes is the registry of probes that auth login runs after the
// token exchange. v1 ships exactly one probe; future per-Unternehmen
// attributes (currency, fiscal-year-start, …) plug in here.
func DefaultProbes() []Probe {
	return []Probe{SKRProbe}
}

// RunProbes executes the supplied probes in order, stopping at the first
// error.
func RunProbes(probes []Probe, ctx *ProbeContext) error {
	for _, probe := range probes {
		if err := probe(ctx); err != nil {
			return err
		}
	}
	return nil
}

// SKRProbe detects the chart-of-accounts standard. It first honours an
// explicit --skr flag (non-interactive bypass); otherwise it queries
// /impersonalaccounts for the standard Erlöskonto numbers — 4400 → SKR04,
// 8400 → SKR03 — and falls back to a TTY prompt when the chart is custom.
func SKRProbe(ctx *ProbeContext) error {
	if ctx.SKRFlag != "" {
		switch ctx.SKRFlag {
		case SKR03, SKR04:
			return ctx.Config.Set(ConfigKeySKR, ctx.SKRFlag)
		default:
			return fmt.Errorf("invalid --skr value: %q (expected %s or %s)", ctx.SKRFlag, SKR03, SKR04)
		}
	}

	hit4400, err := skrAccountExists(ctx.Client, "4400")
	if err != nil {
		return err
	}
	if hit4400 {
		return ctx.Config.Set(ConfigKeySKR, SKR04)
	}
	hit8400, err := skrAccountExists(ctx.Client, "8400")
	if err != nil {
		return err
	}
	if hit8400 {
		return ctx.Config.Set(ConfigKeySKR, SKR03)
	}

	if ctx.Prompt == nil {
		// Non-interactive caller and no auto-detect hit; leave SKR unset
		// so accounting commands fail-fast with the standard message.
		return nil
	}
	answer, err := ctx.Prompt("SKR could not be auto-detected. [1] SKR03  [2] SKR04  [3] skip: ")
	if err != nil {
		return err
	}
	switch strings.TrimSpace(answer) {
	case "1":
		return ctx.Config.Set(ConfigKeySKR, SKR03)
	case "2":
		return ctx.Config.Set(ConfigKeySKR, SKR04)
	case "3", "":
		return nil
	default:
		return fmt.Errorf("invalid SKR choice: %q (expected 1, 2, or 3)", answer)
	}
}

func skrAccountExists(client *Client, number string) (bool, error) {
	body := map[string]any{
		"page":     0,
		"pageSize": 1,
		"search": []map[string]any{
			{"field": "number", "operator": string(OpEquals), "value": number},
		},
	}
	raw, err := client.JSON(http.MethodPost, "/impersonalaccounts", body, nil)
	if err != nil {
		return false, err
	}
	records, err := RecordsFromResponse(raw)
	if err != nil {
		return false, err
	}
	return len(records) > 0, nil
}
