package scopeskill

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestParseScopevisioDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2025-01-01T00:00:00.000Z+0100", "2025-01-01T00:00:00+01:00"},
		{"2026-05-07T23:59:59.999Z+0200", "2026-05-07T23:59:59.999+02:00"},
	}
	for _, tc := range cases {
		got, err := parseScopevisioDate(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if got.Format(time.RFC3339Nano) != tc.want && got.Format(time.RFC3339) != tc.want {
			t.Fatalf("%s: parsed = %s want %s", tc.in, got.Format(time.RFC3339Nano), tc.want)
		}
	}
	if _, err := parseScopevisioDate("not-a-date"); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchSaldoFiltersByKontonummerAndPassesDateFormat(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest"+SaldoEndpointSachkonto {
			t.Fatalf("path = %s", r.URL.Path)
		}
		got = r.URL.Query()
		writeJSON(w, map[string]any{
			"records": []any{
				map[string]any{"Kontonummer": "1200", "Saldo": "1,00"},
				map[string]any{"Kontonummer": "4400", "Saldo": "42,00", "Saldo-Kumuliert": "100,00"},
			},
		})
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)

	rec, err := FetchSaldo(client, SaldoEndpointSachkonto, "4400", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if rec["Kontonummer"] != "4400" || rec["Saldo-Kumuliert"] != "100,00" {
		t.Fatalf("rec = %#v", rec)
	}
	if got.Get("startDate") != "01.01.2026" || got.Get("endDate") != "07.05.2026" {
		t.Fatalf("query = %v", got)
	}
}

func TestFetchSaldoReturnsNilWhenAccountAbsent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"records": []any{
			map[string]any{"Kontonummer": "1200", "Saldo": "1,00"},
		}})
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	rec, err := FetchSaldo(client, SaldoEndpointSachkonto, "9999", time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rec != nil {
		t.Fatalf("rec = %#v", rec)
	}
}

func TestFetchSaldoRejectsEmptyAccountNumber(t *testing.T) {
	if _, err := FetchSaldo(nil, SaldoEndpointSachkonto, "", time.Now(), time.Now()); err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchFiscalYearsParsesShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/fiscalyears" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{
			"years": []any{
				map[string]any{"id": 56, "name": "Eröffnungsbilanz", "beginning": "2024-12-31T00:00:00.000Z+0100", "open": true},
				map[string]any{"id": 45, "name": "2025", "beginning": "2025-01-01T00:00:00.000Z+0100", "open": true},
				map[string]any{"id": 58, "name": "2026", "beginning": "2026-01-01T00:00:00.000Z+0100", "open": true},
			},
		})
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, AccessToken: "test"})
	years, err := FetchFiscalYears(client)
	if err != nil {
		t.Fatal(err)
	}
	if len(years) != 3 {
		t.Fatalf("years = %#v", years)
	}
	if years[1].Name != "2025" || years[1].Beginning.Year() != 2025 || years[1].Beginning.Month() != time.January {
		t.Fatalf("years[1] = %#v", years[1])
	}
}

func TestCurrentFiscalYearPicksLatestStarted(t *testing.T) {
	years := []FiscalYear{
		{Name: "Eröffnungsbilanz", Beginning: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
		{Name: "2025", Beginning: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Name: "2026", Beginning: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Name: "2027", Beginning: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	fy, ok := CurrentFiscalYear(years, now)
	if !ok || fy.Name != "2026" {
		t.Fatalf("fy = %#v ok=%v", fy, ok)
	}
}

func TestCurrentFiscalYearReturnsFalseWhenAllFuture(t *testing.T) {
	years := []FiscalYear{
		{Name: "2030", Beginning: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	if _, ok := CurrentFiscalYear(years, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Fatal("expected ok=false")
	}
}

func TestEarliestFiscalYear(t *testing.T) {
	years := []FiscalYear{
		{Name: "2025", Beginning: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Name: "Eröffnung", Beginning: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	fy, ok := EarliestFiscalYear(years)
	if !ok || fy.Name != "Eröffnung" {
		t.Fatalf("fy = %#v ok=%v", fy, ok)
	}
}

func TestFiscalYearEndDerivesFromNextYear(t *testing.T) {
	years := []FiscalYear{
		{Name: "2025", Beginning: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Name: "2026", Beginning: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	end := FiscalYearEnd(years, years[0])
	want := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	if !end.Equal(want) {
		t.Fatalf("end = %s want %s", end, want)
	}
}

func TestFiscalYearEndFallsBackWhenNoLaterYear(t *testing.T) {
	years := []FiscalYear{
		{Name: "2026", Beginning: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	end := FiscalYearEnd(years, years[0])
	if end.Year() != 2026 || end.Month() != time.December || end.Day() != 31 {
		t.Fatalf("end = %s", end)
	}
}

// silences unused-import flag when reflect isn't otherwise used.
var _ = reflect.DeepEqual
