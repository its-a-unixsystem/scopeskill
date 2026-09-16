package scopeskill

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToPostingsRequestExpandsSharedFields(t *testing.T) {
	raw := `{
		"documentNumber": "P-2025-1",
		"postingDate": "2025-06-02",
		"documentText": "Wareneingang",
		"autoCreateTax": false,
		"rows": [
			{"account": "4400", "amount": 119.00, "vatKey": "U19"},
			{"account": "1200", "amount": -119.00}
		]
	}`
	in, err := ParseSinglePostingInput([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	req, err := in.ToPostingsRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Rows) != 2 {
		t.Fatalf("rows = %#v", req.Rows)
	}
	for i, row := range req.Rows {
		if row.PostingDate != 1748822400000 {
			t.Fatalf("rows[%d].postingDate = %d", i, row.PostingDate)
		}
		if row.DocumentNumber != "P-2025-1" || row.DocumentText != "Wareneingang" {
			t.Fatalf("rows[%d] = %#v", i, row)
		}
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"amount":119.00`, `"amount":-119.00`, `"autoCreateTax":false`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("request JSON missing %s in %s", want, out)
		}
	}
}

func TestToPostingsRequestOmitsAutoCreateTaxWhenUnset(t *testing.T) {
	raw := `{
		"documentNumber": "P-2025-2",
		"postingDate": "2025-06-02",
		"rows": [
			{"account": "4400", "amount": "119.00"},
			{"account": "1200", "amount": "-119.00"}
		]
	}`
	in, err := ParseSinglePostingInput([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	req, err := in.ToPostingsRequest()
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "autoCreateTax") || strings.Contains(string(out), "adjustVatKey") {
		t.Fatalf("request JSON leaks unset flags: %s", out)
	}
}

func TestParseSinglePostingInputRejections(t *testing.T) {
	validRows := `"rows":[{"account":"4400","amount":119.00},{"account":"1200","amount":-119.00}]`
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"one row", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":119.00}]}`, "rows"},
		{"zero amount", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":0.00},{"account":"1200","amount":-119.00}]}`, "rows[1]: amount must not be zero"},
		{"three decimals", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":1.005},{"account":"1200","amount":-119.00}]}`, "amount"},
		{"amount not numeric", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":"abc"},{"account":"1200","amount":-119.00}]}`, "amount"},
		{"short date", `{"documentNumber":"P-1","postingDate":"2025-6-2",` + validRows + `}`, "postingDate"},
		{"dotted date", `{"documentNumber":"P-1","postingDate":"02.06.2025",` + validRows + `}`, "postingDate"},
		{"missing documentNumber", `{"postingDate":"2025-06-02",` + validRows + `}`, "documentNumber"},
		{"missing account", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"amount":119.00},{"account":"1200","amount":-119.00}]}`, "rows[1]: account"},
		{"row-level documentNumber", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":119.00,"documentNumber":"P-1"},{"account":"1200","amount":-119.00}]}`, "unknown field"},
		{"cancelDocument", `{"documentNumber":"P-1","postingDate":"2025-06-02","cancelDocument":true,` + validRows + `}`, "unknown field"},
		{"dimensionId kostenstelle", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":119.00,"dimensions":[{"dimensionId":"kostenstelle","dimensionAccountNumber":"111"}]},{"account":"1200","amount":-119.00}]}`, "dimensionId"},
		{"dimensionId out of range", `{"documentNumber":"P-1","postingDate":"2025-06-02","rows":[{"account":"4400","amount":119.00,"dimensions":[{"dimensionId":"dimension_11","dimensionAccountNumber":"111"}]},{"account":"1200","amount":-119.00}]}`, "dimensionId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSinglePostingInput([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestAmountParseAndFormat(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50", "50.00"},
		{"0.5", "0.50"},
		{"-119.99", "-119.99"},
		{"119", "119.00"},
	}
	for _, tc := range cases {
		amount, err := ParseAmount(tc.in)
		if err != nil {
			t.Fatalf("ParseAmount(%q): %v", tc.in, err)
		}
		if amount.String() != tc.want {
			t.Fatalf("ParseAmount(%q).String() = %q, want %q", tc.in, amount.String(), tc.want)
		}
	}
	for _, bad := range []string{"1e2", "1.005", "abc", "", "1,5"} {
		if _, err := ParseAmount(bad); err == nil {
			t.Fatalf("ParseAmount(%q) unexpectedly succeeded", bad)
		}
	}
}

func TestAmountUnmarshalJSON(t *testing.T) {
	var amount Amount
	if err := json.Unmarshal([]byte(`"12.34"`), &amount); err != nil {
		t.Fatal(err)
	}
	if amount.String() != "12.34" {
		t.Fatalf("amount = %q", amount.String())
	}
	if err := json.Unmarshal([]byte(`12.34`), &amount); err != nil {
		t.Fatal(err)
	}
	if amount.String() != "12.34" {
		t.Fatalf("amount = %q", amount.String())
	}
	for _, bad := range []string{`1e2`, `1.005`, `"abc"`} {
		if err := json.Unmarshal([]byte(bad), &amount); err == nil {
			t.Fatalf("UnmarshalJSON(%s) unexpectedly succeeded", bad)
		}
	}
}

func TestISODateMillis(t *testing.T) {
	ms, err := ISODateMillis("2025-06-02")
	if err != nil {
		t.Fatal(err)
	}
	if ms != 1748822400000 {
		t.Fatalf("ms = %d", ms)
	}
	if _, err := ISODateMillis("2025-6-2"); err == nil {
		t.Fatal("expected error")
	}
}

const canonicalInputJSON = `{
	"documentNumber": "P-2025-1",
	"postingDate": "2025-06-02",
	"rows": [
		{"account": "4400", "amount": 119.00, "vatKey": "U19", "dimensions": [{"dimensionId": "dimension_1", "dimensionAccountNumber": "111"}]},
		{"account": "1200", "amount": -119.00}
	]
}`

func canonicalFixture(t *testing.T) CanonicalBuchung {
	t.Helper()
	in, err := ParseSinglePostingInput([]byte(canonicalInputJSON))
	if err != nil {
		t.Fatal(err)
	}
	return in.Canonical()
}

func TestCanonicalFromJournalMatchesInput(t *testing.T) {
	expected := canonicalFixture(t)
	records := []any{
		map[string]any{
			"documentNumber":      "P-2025-1",
			"postingDate":         float64(1748822400000),
			"accountNumber":       "4400",
			"debitAmount":         119.0,
			"creditAmount":        0.0,
			"vatKey":              "U19",
			"documentDimension_1": 111.0,
		},
		map[string]any{
			"documentNumber": "P-2025-1",
			"postingDate":    "2025-06-02",
			"accountNumber":  "1200",
			"debitAmount":    0.0,
			"creditAmount":   119.0,
		},
	}
	actual, err := CanonicalFromJournal(records)
	if err != nil {
		t.Fatal(err)
	}
	diff := CompareBuchung(expected, actual, false)
	if !diff.Equal() {
		out, _ := json.MarshalIndent(diff, "", "  ")
		t.Fatalf("diff = %s", out)
	}
}

func TestCompareBuchungAmountMismatch(t *testing.T) {
	expected := canonicalFixture(t)
	actual, err := CanonicalFromJournal([]any{
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 100.0, "creditAmount": 0.0, "vatKey": "U19", "documentDimension_1": 111.0},
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0},
	})
	if err != nil {
		t.Fatal(err)
	}
	diff := CompareBuchung(expected, actual, false)
	if diff.Equal() {
		t.Fatal("expected non-equal diff")
	}
	if len(diff.Missing) != 1 || diff.Missing[0].Amount != "119.00" {
		t.Fatalf("missing = %#v", diff.Missing)
	}
	if len(diff.Unexpected) != 1 || diff.Unexpected[0].Amount != "100.00" {
		t.Fatalf("unexpected = %#v", diff.Unexpected)
	}
}

func TestCompareBuchungGeneratedTaxRow(t *testing.T) {
	expected := canonicalFixture(t)
	actual, err := CanonicalFromJournal([]any{
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "4400", "debitAmount": 119.0, "creditAmount": 0.0, "vatKey": "U19", "documentDimension_1": 111.0},
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "1200", "debitAmount": 0.0, "creditAmount": 119.0},
		map[string]any{"documentNumber": "P-2025-1", "postingDate": float64(1748822400000), "accountNumber": "3806", "debitAmount": 0.0, "creditAmount": 19.0, "vatKey": "U19"},
	})
	if err != nil {
		t.Fatal(err)
	}
	diff := CompareBuchung(expected, actual, true)
	if !diff.Equal() {
		t.Fatalf("diff = %#v", diff)
	}
	if len(diff.Generated) != 1 || diff.Generated[0].Account != "3806" {
		t.Fatalf("generated = %#v", diff.Generated)
	}
	diff = CompareBuchung(expected, actual, false)
	if diff.Equal() || len(diff.Unexpected) != 1 {
		t.Fatalf("diff = %#v", diff)
	}
}

func TestCanonicalFromJournalRejectsEmpty(t *testing.T) {
	if _, err := CanonicalFromJournal(nil); err == nil {
		t.Fatal("expected error")
	}
}
