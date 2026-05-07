package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/its-a-unixsystem/scopeskill/internal/scopeskill"
)

type kontaktStub struct {
	server        *httptest.Server
	searchBodies  []map[string]any
	contactRoutes []string
	contactFields []url.Values
	searchPages   [][]any
	idx           int
	contactByID   map[string]map[string]any
	contactStatus int
}

func newKontaktStub(t *testing.T) *kontaktStub {
	t.Helper()
	stub := &kontaktStub{
		contactByID:   map[string]map[string]any{},
		contactStatus: http.StatusOK,
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/token":
			writeJSONForCLI(w, map[string]any{
				"token_type":   "Bearer",
				"access_token": "access-from-refresh",
				"expires_in":   3600,
			})
		case r.URL.Path == "/rest/contacts" && r.Method == http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			stub.searchBodies = append(stub.searchBodies, body)
			page := []any{}
			if stub.idx < len(stub.searchPages) {
				page = stub.searchPages[stub.idx]
				stub.idx++
			}
			writeJSONForCLI(w, map[string]any{"records": page})
		case strings.HasPrefix(r.URL.Path, "/rest/contact/"):
			stub.contactRoutes = append(stub.contactRoutes, r.URL.Path)
			stub.contactFields = append(stub.contactFields, r.URL.Query())
			id := strings.TrimPrefix(r.URL.Path, "/rest/contact/")
			rec, ok := stub.contactByID[id]
			if !ok {
				http.Error(w, "missing", stub.contactStatus)
				return
			}
			writeJSONForCLI(w, rec)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func kontaktConfigPath(t *testing.T, baseURL string) string {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config")
	configRaw := []byte("BASE_URL=" + baseURL + "\nCUSTOMER=1234567\nREST_REFRESH_TOKEN=refresh-token\n")
	if err := os.WriteFile(configPath, configRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(scopeskill.EnvAccessTokenCache, filepath.Join(t.TempDir(), "access-token.json"))
	return configPath
}

func TestKontaktSearchByNameUsesContainsOnLastname(t *testing.T) {
	stub := newKontaktStub(t)
	stub.searchPages = [][]any{{
		map[string]any{"id": 49189.0, "lastname": "Ingenire UG (haftungsbeschränkt)", "vatId": "DE366054310"},
	}}
	configPath := kontaktConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kontakt", "search", "--name=Ingenire"}); err != nil {
		t.Fatal(err)
	}
	if len(stub.searchBodies) != 1 {
		t.Fatalf("search bodies = %d", len(stub.searchBodies))
	}
	body := stub.searchBodies[0]
	if body["pageSize"].(float64) != 100 {
		t.Fatalf("pageSize = %v", body["pageSize"])
	}
	cond := body["search"].([]any)[0].(map[string]any)
	if cond["field"] != "lastname" || cond["operator"] != "contains" || cond["value"] != "Ingenire" {
		t.Fatalf("condition = %#v", cond)
	}

	var got []any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("stdout JSON: %v: %s", err, output.String())
	}
	if len(got) != 1 || got[0].(map[string]any)["lastname"] != "Ingenire UG (haftungsbeschränkt)" {
		t.Fatalf("records = %#v", got)
	}
}

func TestKontaktSearchByUstIDAndEmail(t *testing.T) {
	stub := newKontaktStub(t)
	stub.searchPages = [][]any{{}}
	configPath := kontaktConfigPath(t, stub.server.URL)
	withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kontakt", "search", "--ust-id=DE366054310", "--email=ingenire.com"}); err != nil {
		t.Fatal(err)
	}
	body := stub.searchBodies[0]
	conds := body["search"].([]any)
	if len(conds) != 2 {
		t.Fatalf("conditions = %#v", conds)
	}
	first := conds[0].(map[string]any)
	if first["field"] != "vatId" || first["operator"] != "equals" || first["value"] != "DE366054310" {
		t.Fatalf("ust-id condition = %#v", first)
	}
	second := conds[1].(map[string]any)
	if second["field"] != "email" || second["operator"] != "contains" || second["value"] != "ingenire.com" {
		t.Fatalf("email condition = %#v", second)
	}
}

func TestKontaktShowStitchesDebitorAndKreditorWhenLinked(t *testing.T) {
	stub := newKontaktStub(t)
	stub.contactByID["49191"] = map[string]any{
		"id":             49191.0,
		"lastname":       "Kunden A-Z",
		"debitorNumber":  "10000",
		"kreditorNumber": nil,
	}
	stub.contactByID["49193"] = map[string]any{
		"id":             49193.0,
		"lastname":       "Sixt GmbH & Co. Autovermietung KG",
		"debitorNumber":  nil,
		"kreditorNumber": "70001",
	}
	stub.contactByID["49195"] = map[string]any{
		"id":             49195.0,
		"lastname":       "Both",
		"debitorNumber":  "10005",
		"kreditorNumber": "70005",
	}
	configPath := kontaktConfigPath(t, stub.server.URL)

	cases := []struct {
		id             string
		wantDebitor    bool
		wantKreditor   bool
		wantDebitorNo  string
		wantKreditorNo string
	}{
		{"49191", true, false, "10000", ""},
		{"49193", false, true, "", "70001"},
		{"49195", true, true, "10005", "70005"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			output, _ := withCLI(t, "", false)
			if err := run([]string{"--config", configPath, "kontakt", "show", tc.id}); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatalf("stdout JSON: %v: %s", err, output.String())
			}
			kontakt := got["kontakt"].(map[string]any)
			if kontakt == nil {
				t.Fatalf("missing kontakt: %s", output.String())
			}
			deb, hasDeb := got["debitor"]
			if hasDeb != tc.wantDebitor {
				t.Fatalf("debitor key present=%v want=%v in %s", hasDeb, tc.wantDebitor, output.String())
			}
			if tc.wantDebitor && deb.(map[string]any)["number"] != tc.wantDebitorNo {
				t.Fatalf("debitor.number = %v", deb)
			}
			kre, hasKre := got["kreditor"]
			if hasKre != tc.wantKreditor {
				t.Fatalf("kreditor key present=%v want=%v in %s", hasKre, tc.wantKreditor, output.String())
			}
			if tc.wantKreditor && kre.(map[string]any)["number"] != tc.wantKreditorNo {
				t.Fatalf("kreditor.number = %v", kre)
			}
		})
	}
}

func TestKontaktShowOmitsBothLinksWhenKontaktUnlinked(t *testing.T) {
	stub := newKontaktStub(t)
	stub.contactByID["49189"] = map[string]any{
		"id":             49189.0,
		"lastname":       "Ingenire UG (haftungsbeschränkt)",
		"debitorNumber":  nil,
		"kreditorNumber": nil,
	}
	configPath := kontaktConfigPath(t, stub.server.URL)
	output, _ := withCLI(t, "", false)

	if err := run([]string{"--config", configPath, "kontakt", "show", "49189"}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["debitor"]; ok {
		t.Fatalf("expected no debitor key: %s", output.String())
	}
	if _, ok := got["kreditor"]; ok {
		t.Fatalf("expected no kreditor key: %s", output.String())
	}
	if _, ok := got["kontakt"]; !ok {
		t.Fatalf("missing kontakt key: %s", output.String())
	}
}

func TestKontaktShowReturnsNotFoundFor400(t *testing.T) {
	stub := newKontaktStub(t)
	stub.contactStatus = http.StatusBadRequest
	configPath := kontaktConfigPath(t, stub.server.URL)
	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "kontakt", "show", "999999"})
	if err == nil || !strings.Contains(err.Error(), "not found or authorization missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestKontaktSearchDataWithAllIsRejected(t *testing.T) {
	stub := newKontaktStub(t)
	configPath := kontaktConfigPath(t, stub.server.URL)
	withCLI(t, "", false)
	err := run([]string{"--config", configPath, "kontakt", "search", "--data", `{"page":0,"pageSize":1}`, "--all"})
	if err == nil || !strings.Contains(err.Error(), "--data cannot be combined") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.searchBodies) != 0 {
		t.Fatalf("no requests should have been made, got %d", len(stub.searchBodies))
	}
}

func TestKontaktWithoutSubcommandPrintsHelpAndFails(t *testing.T) {
	output, _ := withCLI(t, "", false)
	client := scopeskill.NewClient(scopeskill.Config{AccessToken: "access-token"})
	err := kontakt(client, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"search", "show"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("missing %q in %q", want, output.String())
		}
	}
}
