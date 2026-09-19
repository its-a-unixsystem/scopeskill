package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

// TestLookupCommandCoversEveryDispatchedName ensures the command table
// contains every top-level name the CLI is expected to accept, so the
// table never silently drops a command that usage() advertises.
func TestLookupCommandCoversEveryDispatchedName(t *testing.T) {
	want := []string{
		"auth", "help",
		"get", "post", "download", "datev", "bericht", "teamwork",
		"sachkonto", "kontakt", "debitor", "kreditor", "personenkonto",
		"buchhaltung", "dimension", "textbaustein", "statistik",
		"zahlungsbedingung", "steuermatrix", "steuersachverhalt",
		"eingangsrechnung", "gutschrift", "offene-posten", "journal",
		"kasse", "reisekosten", "buchung",
	}
	for _, name := range want {
		if _, ok := lookupCommand(name); !ok {
			t.Errorf("lookupCommand(%q) = false, want true", name)
		}
	}
}

// TestRunUnknownCommandPreservesErrorText locks the exact wording the
// CLI returns for an unknown top-level command.
func TestRunUnknownCommandPreservesErrorText(t *testing.T) {
	output, _ := withCLI(t, "", false)
	err := run([]string{"definitely-not-a-command"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if got := err.Error(); got != "unknown command: definitely-not-a-command" {
		t.Fatalf("unknown command error = %q, want %q", got, "unknown command: definitely-not-a-command")
	}
	if output.Len() != 0 {
		t.Fatalf("unknown command wrote to stdout: %q", output.String())
	}
}

// TestRunUnknownCommandDoesNotReadConfig proves lazy dispatch semantics:
// an unknown command must fail at command lookup before any config is
// loaded. The config file exists and is malformed, so an eager config
// read would surface the parse error instead of the unknown-command
// error.
func TestRunUnknownCommandDoesNotReadConfig(t *testing.T) {
	const badLine = "BAD LINE"
	malformed := filepath.Join(t.TempDir(), "malformed.env")
	if err := os.WriteFile(malformed, []byte(badLine+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output, _ := withCLI(t, "", false)
	err := run([]string{"--config", malformed, "no-such-command"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if got := err.Error(); got != "unknown command: no-such-command" {
		t.Fatalf("unknown command error = %q, want %q", got, "unknown command: no-such-command")
	}
	if output.Len() != 0 {
		t.Fatalf("unknown command wrote to stdout: %q", output.String())
	}
	// The malformed file must be untouched: dispatch never read or rewrote it.
	raw, readErr := os.ReadFile(malformed)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != badLine+"\n" {
		t.Fatalf("config file mutated by failed dispatch: %q", string(raw))
	}
}

// TestRunHelpAliasesAllProduceUsage proves help/-h/--help all route
// to the usage banner. -h and --help are intercepted by flag.Parse
// as flag.ErrHelp; run() maps that to usage() rather than leaking the
// flag error to stderr.
func TestRunHelpAliasesAllProduceUsage(t *testing.T) {
	for _, name := range []string{"help", "-h", "--help"} {
		t.Run(name, func(t *testing.T) {
			output, stderr := withCLI(t, "", false)
			err := run([]string{name})
			if err != nil {
				t.Fatalf("run(%q) error: %v", name, err)
			}
			if !strings.HasPrefix(output.String(), "usage: sv-cli") {
				t.Fatalf("run(%q) stdout = %q, want usage banner", name, output.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("run(%q) wrote to stderr: %q", name, stderr.String())
			}
		})
	}
}

// TestRunInvalidGlobalFlagStillErrors proves that suppressing the
// flag package's default help listing did not swallow real parse
// errors: an unknown global flag must fail with a non-nil error
// whose message names the offending flag.
func TestRunInvalidGlobalFlagStillErrors(t *testing.T) {
	_, stderr := withCLI(t, "", false)
	err := run([]string{"--definitely-invalid"})
	if err == nil {
		t.Fatal("expected error for unknown global flag, got nil")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -definitely-invalid") {
		t.Fatalf("invalid-flag error = %q, want it to name the offending flag", err.Error())
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -definitely-invalid") {
		t.Fatalf("stderr = %q, want the parse error surfaced on stderr", stderr.String())
	}
}

// TestRunNoArgsPrintsUsage mirrors the zero-argument path that run()
// hits before any command lookup.
func TestRunNoArgsPrintsUsage(t *testing.T) {
	output, _ := withCLI(t, "", false)
	if err := run(nil); err != nil {
		t.Fatalf("run(nil) error: %v", err)
	}
	if !strings.HasPrefix(output.String(), "usage: sv-cli") {
		t.Fatalf("run(nil) stdout = %q, want usage banner", output.String())
	}
}

// TestRunAuthDoesNotConstructClient proves auth is dispatched without
// client construction: with no token configured, auth show must report
// the missing-token error rather than a config-load or HTTP failure.
func TestRunAuthDoesNotConstructClient(t *testing.T) {
	t.Setenv(scopeskill.EnvRestRefreshToken, "")
	path := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(path, []byte("CUSTOMER=1234567\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr := withCLI(t, "", false)
	err := run([]string{"--config", path, "auth", "show"})
	if err == nil || !strings.Contains(err.Error(), "missing REST refresh token") {
		t.Fatalf("auth show with empty config = %v, want missing-token error", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("auth show wrote to stderr before token check: %q", stderr.String())
	}
}
