package scopeskill

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SuSa endpoints exposed by Scopevisio. The same response shape
// (`{records: [{Kontonummer, Soll, Haben, Saldenvortrag, Saldo,
// Soll-Kumuliert, Haben-Kumuliert, Saldo-Kumuliert, Kontoname}]}`) is
// returned by all three.
//
// Reused by sachkonto show|balance (slice 2) and debitor|kreditor
// show|balance (slice 5).
const (
	SaldoEndpointSachkonto = "/datasource/susa/impersonalAccounts"
	SaldoEndpointDebitor   = "/datasource/susa/debtors"
	SaldoEndpointKreditor  = "/datasource/susa/creditors"

	saldoDateFormat = "02.01.2006"
)

// FetchSaldo returns the SuSa row for the supplied accountNumber over the
// [from, to] range, or nil when the account does not appear in the response.
// Filtering is done client-side because the upstream
// `accountingEntityNumbers` query parameter scopes Buchungskreise, not Konten.
//
// Endpoint must be one of SaldoEndpointSachkonto, SaldoEndpointDebitor, or
// SaldoEndpointKreditor.
func FetchSaldo(client *Client, endpoint string, accountNumber string, from, to time.Time) (map[string]any, error) {
	if accountNumber == "" {
		return nil, errors.New("FetchSaldo: accountNumber is required")
	}
	query := map[string]string{
		"startDate": from.Format(saldoDateFormat),
		"endDate":   to.Format(saldoDateFormat),
	}
	raw, err := client.JSON(http.MethodGet, endpoint, nil, query)
	if err != nil {
		return nil, err
	}
	return saldoRecordFor(raw, accountNumber), nil
}

func saldoRecordFor(raw any, accountNumber string) map[string]any {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	records, ok := obj["records"].([]any)
	if !ok {
		return nil
	}
	for _, r := range records {
		rec, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := rec["Kontonummer"].(string); ok && v == accountNumber {
			return rec
		}
	}
	return nil
}

// FiscalYear is the projection of one /fiscalyears entry that accounting
// commands need to derive default Saldo ranges.
type FiscalYear struct {
	ID        int
	Name      string
	Beginning time.Time
	Open      bool
}

// FetchFiscalYears returns the Mandant's fiscal years, sorted by Beginning.
func FetchFiscalYears(client *Client) ([]FiscalYear, error) {
	raw, err := client.JSON(http.MethodGet, "/fiscalyears", nil, nil)
	if err != nil {
		return nil, err
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("/fiscalyears response is not an object")
	}
	list, _ := obj["years"].([]any)
	out := make([]FiscalYear, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		fy := FiscalYear{}
		if v, ok := m["id"].(float64); ok {
			fy.ID = int(v)
		}
		if v, ok := m["name"].(string); ok {
			fy.Name = v
		}
		if v, ok := m["open"].(bool); ok {
			fy.Open = v
		}
		if v, ok := m["beginning"].(string); ok {
			if t, err := parseScopevisioDate(v); err == nil {
				fy.Beginning = t
			}
		}
		out = append(out, fy)
	}
	return out, nil
}

// CurrentFiscalYear picks the latest fiscal year whose Beginning is on or
// before now. Returns false when no candidate exists.
func CurrentFiscalYear(years []FiscalYear, now time.Time) (FiscalYear, bool) {
	var best FiscalYear
	found := false
	for _, fy := range years {
		if fy.Beginning.IsZero() || fy.Beginning.After(now) {
			continue
		}
		if !found || fy.Beginning.After(best.Beginning) {
			best = fy
			found = true
		}
	}
	return best, found
}

// EarliestFiscalYear returns the year with the earliest Beginning.
func EarliestFiscalYear(years []FiscalYear) (FiscalYear, bool) {
	var best FiscalYear
	found := false
	for _, fy := range years {
		if fy.Beginning.IsZero() {
			continue
		}
		if !found || fy.Beginning.Before(best.Beginning) {
			best = fy
			found = true
		}
	}
	return best, found
}

// FiscalYearEnd derives the end-of-year date from the next fiscal year's
// Beginning. When no later year exists, falls back to one year after fy's
// Beginning. The returned instant is one second before the next year starts.
func FiscalYearEnd(years []FiscalYear, fy FiscalYear) time.Time {
	var nextBeg time.Time
	for _, candidate := range years {
		if candidate.Beginning.IsZero() || !candidate.Beginning.After(fy.Beginning) {
			continue
		}
		if nextBeg.IsZero() || candidate.Beginning.Before(nextBeg) {
			nextBeg = candidate.Beginning
		}
	}
	if nextBeg.IsZero() {
		return fy.Beginning.AddDate(1, 0, 0).Add(-time.Second)
	}
	return nextBeg.Add(-time.Second)
}

// parseScopevisioDate parses date strings like "2025-01-01T00:00:00.000Z+0100"
// — note the stray "Z" before the timezone offset, which is not standard
// ISO 8601.
func parseScopevisioDate(s string) (time.Time, error) {
	cleaned := strings.Replace(s, "Z+", "+", 1)
	cleaned = strings.Replace(cleaned, "Z-", "-", 1)
	if t, err := time.Parse("2006-01-02T15:04:05.000-0700", cleaned); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse Scopevisio date: %q", s)
}
