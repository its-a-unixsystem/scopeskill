package scopeskill

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func skrServer(t *testing.T, hits map[string]bool) (*httptest.Server, *[]string) {
	t.Helper()
	queried := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/impersonalaccounts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		search := body["search"].([]any)
		cond := search[0].(map[string]any)
		number := cond["value"].(string)
		*queried = append(*queried, number)
		if hits[number] {
			writeJSON(w, []any{map[string]any{"id": 1, "number": number}})
			return
		}
		writeJSON(w, []any{})
	}))
	t.Cleanup(server.Close)
	return server, queried
}

func freshConfigFile() *ConfigFile {
	cf := &ConfigFile{values: map[string]string{}}
	return cf
}

func TestSKRProbe4400HitsSKR04(t *testing.T) {
	server, queried := skrServer(t, map[string]bool{"4400": true})
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
		Config: cf,
	}
	if err := SKRProbe(ctx); err != nil {
		t.Fatal(err)
	}
	if cf.Values()[ConfigKeySKR] != SKR04 {
		t.Fatalf("SKR = %q", cf.Values()[ConfigKeySKR])
	}
	if len(*queried) != 1 || (*queried)[0] != "4400" {
		t.Fatalf("queried = %v", *queried)
	}
}

func TestSKRProbe8400HitsSKR03(t *testing.T) {
	server, queried := skrServer(t, map[string]bool{"8400": true})
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
		Config: cf,
	}
	if err := SKRProbe(ctx); err != nil {
		t.Fatal(err)
	}
	if cf.Values()[ConfigKeySKR] != SKR03 {
		t.Fatalf("SKR = %q", cf.Values()[ConfigKeySKR])
	}
	want := []string{"4400", "8400"}
	if len(*queried) != 2 || (*queried)[0] != want[0] || (*queried)[1] != want[1] {
		t.Fatalf("queried = %v", *queried)
	}
}

func TestSKRProbeFallsBackToPromptWhenNeitherMatches(t *testing.T) {
	server, queried := skrServer(t, map[string]bool{})
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{name: "user picks skr03", answer: "1", want: SKR03},
		{name: "user picks skr04", answer: "2", want: SKR04},
		{name: "user picks skip", answer: "3", want: ""},
		{name: "empty answer treated as skip", answer: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cf := freshConfigFile()
			var asked string
			ctx := &ProbeContext{
				Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
				Config: cf,
				Prompt: func(message string) (string, error) {
					asked = message
					return tc.answer, nil
				},
			}
			if err := SKRProbe(ctx); err != nil {
				t.Fatal(err)
			}
			if got := cf.Values()[ConfigKeySKR]; got != tc.want {
				t.Fatalf("SKR = %q want %q", got, tc.want)
			}
			if !strings.Contains(asked, "[1] SKR03") || !strings.Contains(asked, "[2] SKR04") || !strings.Contains(asked, "[3] skip") {
				t.Fatalf("prompt = %q", asked)
			}
		})
	}
	if len(*queried) < 2 {
		t.Fatalf("queried = %v", *queried)
	}
}

func TestSKRProbeRejectsInvalidPromptAnswer(t *testing.T) {
	server, _ := skrServer(t, map[string]bool{})
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
		Config: cf,
		Prompt: func(string) (string, error) { return "yes", nil },
	}
	if err := SKRProbe(ctx); err == nil || !strings.Contains(err.Error(), "invalid SKR choice") {
		t.Fatalf("err = %v", err)
	}
}

func TestSKRProbeFlagBypassesAPI(t *testing.T) {
	server, queried := skrServer(t, map[string]bool{})
	for _, value := range []string{SKR03, SKR04} {
		cf := freshConfigFile()
		ctx := &ProbeContext{
			Client:  NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
			Config:  cf,
			SKRFlag: value,
		}
		if err := SKRProbe(ctx); err != nil {
			t.Fatal(err)
		}
		if cf.Values()[ConfigKeySKR] != value {
			t.Fatalf("SKR = %q", cf.Values()[ConfigKeySKR])
		}
	}
	if len(*queried) != 0 {
		t.Fatalf("--skr should bypass the API, but %d queries were made: %v", len(*queried), *queried)
	}
}

func TestSKRProbeFlagRejectsUnknownValue(t *testing.T) {
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client:  NewClient(Config{AccessToken: "test"}),
		Config:  cf,
		SKRFlag: "skr99",
	}
	if err := SKRProbe(ctx); err == nil || !strings.Contains(err.Error(), "invalid --skr") {
		t.Fatalf("err = %v", err)
	}
}

func TestSKRProbeWithoutPromptLeavesValueUnsetWhenNoMatch(t *testing.T) {
	server, _ := skrServer(t, map[string]bool{})
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
		Config: cf,
		// no Prompt, no SKRFlag — non-interactive caller
	}
	if err := SKRProbe(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cf.Values()[ConfigKeySKR]; got != "" {
		t.Fatalf("SKR = %q want empty", got)
	}
}

func TestSKRProbePropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kaboom", http.StatusInternalServerError)
	}))
	defer server.Close()
	cf := freshConfigFile()
	ctx := &ProbeContext{
		Client: NewClient(Config{BaseURL: server.URL, AccessToken: "test"}),
		Config: cf,
	}
	if err := SKRProbe(ctx); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunProbesShortCircuitsOnError(t *testing.T) {
	cf := freshConfigFile()
	calls := 0
	first := Probe(func(*ProbeContext) error {
		calls++
		return errors.New("nope")
	})
	second := Probe(func(*ProbeContext) error {
		calls++
		return nil
	})
	if err := RunProbes([]Probe{first, second}, &ProbeContext{Config: cf}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestRequireSKR(t *testing.T) {
	if err := RequireSKR(map[string]string{}); err == nil {
		t.Fatal("expected error when SKR is missing")
	} else if !strings.Contains(err.Error(), "SKR not configured") {
		t.Fatalf("err = %v", err)
	}
	if err := RequireSKR(map[string]string{ConfigKeySKR: SKR04}); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestNoSKREnvOverride(t *testing.T) {
	t.Setenv("SCOPESKILL_SKR", "skr03")
	t.Setenv("SCOPESKILL_"+ConfigKeySKR, "skr03")
	cf, err := ReadConfigFile(t.TempDir() + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	if got := cf.Values()[ConfigKeySKR]; got != "" {
		t.Fatalf("env override leaked: SKR = %q", got)
	}
	cfg, err := LoadClientConfig("")
	if err != nil {
		t.Fatal(err)
	}
	// Config has no SKR field; ensure no field reads from SCOPESKILL_SKR.
	_ = cfg
}

