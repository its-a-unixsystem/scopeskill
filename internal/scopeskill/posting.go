package scopeskill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SinglePostingInput is the CLI-owned schema for `sv-cli buchung create`: one
// Buchung with shared document fields and at least two rows.
type SinglePostingInput struct {
	DocumentNumber         string             `json:"documentNumber"`
	PostingDate            string             `json:"postingDate"`
	DocumentDate           string             `json:"documentDate,omitempty"`
	ExternalDocumentNumber string             `json:"externalDocumentNumber,omitempty"`
	InternalDocumentNumber string             `json:"internalDocumentNumber,omitempty"`
	DocumentText           string             `json:"documentText,omitempty"`
	AutoCreateTax          *bool              `json:"autoCreateTax,omitempty"`
	AdjustVatKey           *bool              `json:"adjustVatKey,omitempty"`
	Rows                   []SinglePostingRow `json:"rows"`
}

type SinglePostingRow struct {
	Account               string             `json:"account"`
	SummaryAccount        string             `json:"summaryAccount,omitempty"`
	Amount                Amount             `json:"amount"`
	RowText               string             `json:"rowText,omitempty"`
	VatKey                string             `json:"vatKey,omitempty"`
	Dimensions            []PostingDimension `json:"dimensions,omitempty"`
	ForeignCurrencyAmount json.Number        `json:"foreignCurrencyAmount,omitempty"`
	ForeignCurrencyCode   string             `json:"foreignCurrencyCode,omitempty"`
	ForeignCurrencyRate   json.Number        `json:"foreignCurrencyRate,omitempty"`
	DiscountPercent1      json.Number        `json:"discountPercent1,omitempty"`
	DiscountPeriod1       *int64             `json:"discountPeriod1,omitempty"`
	DiscountPercent2      json.Number        `json:"discountPercent2,omitempty"`
	DiscountPeriod2       *int64             `json:"discountPeriod2,omitempty"`
	NetTimeLimit          *int64             `json:"netTimeLimit,omitempty"`
	DiscountAccount       string             `json:"discountAccount,omitempty"`
	PaymentType           string             `json:"paymentType,omitempty"`
	UstID                 string             `json:"ustId,omitempty"`
}

type PostingDimension struct {
	DimensionID            string `json:"dimensionId"`
	DimensionAccountNumber string `json:"dimensionAccountNumber"`
}

// Amount is a EUR amount with at most two decimals, held as cents. It never
// goes through float64.
type Amount struct {
	cents int64
}

var amountPattern = regexp.MustCompile(`^-?\d+(\.\d{1,2})?$`)

func ParseAmount(s string) (Amount, error) {
	if !amountPattern.MatchString(s) {
		return Amount{}, fmt.Errorf("invalid amount %q", s)
	}
	negative := strings.HasPrefix(s, "-")
	digits := strings.TrimPrefix(s, "-")
	whole, fraction, _ := strings.Cut(digits, ".")
	fraction = (fraction + "00")[:2]
	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return Amount{}, fmt.Errorf("invalid amount %q", s)
	}
	if negative {
		cents = -cents
	}
	return Amount{cents: cents}, nil
}

func (a Amount) IsZero() bool {
	return a.cents == 0
}

func (a Amount) String() string {
	cents := a.cents
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func (a Amount) MarshalJSON() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *Amount) UnmarshalJSON(raw []byte) error {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		amount, err := ParseAmount(s)
		if err != nil {
			return err
		}
		*a = amount
		return nil
	}
	amount, err := ParseAmount(trimmed)
	if err != nil {
		return err
	}
	*a = amount
	return nil
}

// AmountFromFloat converts a journal-side float amount (read path only) back
// into cents.
func AmountFromFloat(f float64) Amount {
	return Amount{cents: int64(math.Round(f * 100))}
}

var dimensionIDPattern = regexp.MustCompile(`^dimension_([1-9]|10)$`)

// ParseSinglePostingInput decodes the --data payload strictly (unknown fields
// and trailing data are rejected) and validates it.
func ParseSinglePostingInput(raw []byte) (SinglePostingInput, error) {
	var in SinglePostingInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&in); err != nil {
		return SinglePostingInput{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return SinglePostingInput{}, errors.New("unexpected trailing data after JSON document")
	}
	if err := in.Validate(); err != nil {
		return SinglePostingInput{}, err
	}
	return in, nil
}

func (in SinglePostingInput) Validate() error {
	if in.DocumentNumber == "" {
		return errors.New("documentNumber is required")
	}
	if _, err := isoDate(in.PostingDate); err != nil {
		return fmt.Errorf("postingDate must be YYYY-MM-DD: %q", in.PostingDate)
	}
	if in.DocumentDate != "" {
		if _, err := isoDate(in.DocumentDate); err != nil {
			return fmt.Errorf("documentDate must be YYYY-MM-DD: %q", in.DocumentDate)
		}
	}
	if len(in.Rows) < 2 {
		return fmt.Errorf("rows must contain at least 2 entries, got %d", len(in.Rows))
	}
	for i, row := range in.Rows {
		if row.Account == "" {
			return fmt.Errorf("rows[%d]: account is required", i+1)
		}
		if row.Amount.IsZero() {
			return fmt.Errorf("rows[%d]: amount must not be zero", i+1)
		}
		for _, dim := range row.Dimensions {
			if !dimensionIDPattern.MatchString(dim.DimensionID) {
				return fmt.Errorf("rows[%d]: invalid dimensionId %q", i+1, dim.DimensionID)
			}
			if dim.DimensionAccountNumber == "" {
				return fmt.Errorf("rows[%d]: dimensionAccountNumber is required for %s", i+1, dim.DimensionID)
			}
		}
	}
	return nil
}

func isoDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// ISODateMillis returns the UTC-midnight millisecond timestamp of a
// YYYY-MM-DD date.
func ISODateMillis(s string) (int64, error) {
	t, err := isoDate(s)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

// PostingsRequest is the live POST /postings/new body.
type PostingsRequest struct {
	AutoCreateTax *bool        `json:"autoCreateTax,omitempty"`
	AdjustVatKey  *bool        `json:"adjustVatKey,omitempty"`
	Rows          []PostingRow `json:"rows"`
}

type PostingRow struct {
	PostingDate            int64              `json:"postingDate"`
	DocumentDate           *int64             `json:"documentDate,omitempty"`
	DocumentNumber         string             `json:"documentNumber"`
	ExternalDocumentNumber string             `json:"externalDocumentNumber,omitempty"`
	InternalDocumentNumber string             `json:"internalDocumentNumber,omitempty"`
	DocumentText           string             `json:"documentText,omitempty"`
	Account                string             `json:"account"`
	SummaryAccount         string             `json:"summaryAccount,omitempty"`
	Amount                 Amount             `json:"amount"`
	RowText                string             `json:"rowText,omitempty"`
	VatKey                 string             `json:"vatKey,omitempty"`
	Dimensions             []PostingDimension `json:"dimensions,omitempty"`
	ForeignCurrencyAmount  json.Number        `json:"foreignCurrencyAmount,omitempty"`
	ForeignCurrencyCode    string             `json:"foreignCurrencyCode,omitempty"`
	ForeignCurrencyRate    json.Number        `json:"foreignCurrencyRate,omitempty"`
	DiscountPercent1       json.Number        `json:"discountPercent1,omitempty"`
	DiscountPeriod1        *int64             `json:"discountPeriod1,omitempty"`
	DiscountPercent2       json.Number        `json:"discountPercent2,omitempty"`
	DiscountPeriod2        *int64             `json:"discountPeriod2,omitempty"`
	NetTimeLimit           *int64             `json:"netTimeLimit,omitempty"`
	DiscountAccount        string             `json:"discountAccount,omitempty"`
	PaymentType            string             `json:"paymentType,omitempty"`
	UstID                  string             `json:"ustId,omitempty"`
}

// ToPostingsRequest expands the shared document fields into every row.
// autoCreateTax and adjustVatKey pass through verbatim.
func (in SinglePostingInput) ToPostingsRequest() (PostingsRequest, error) {
	postingDate, err := ISODateMillis(in.PostingDate)
	if err != nil {
		return PostingsRequest{}, err
	}
	var documentDate *int64
	if in.DocumentDate != "" {
		ms, err := ISODateMillis(in.DocumentDate)
		if err != nil {
			return PostingsRequest{}, err
		}
		documentDate = &ms
	}
	req := PostingsRequest{
		AutoCreateTax: in.AutoCreateTax,
		AdjustVatKey:  in.AdjustVatKey,
		Rows:          make([]PostingRow, 0, len(in.Rows)),
	}
	for _, row := range in.Rows {
		req.Rows = append(req.Rows, PostingRow{
			PostingDate:            postingDate,
			DocumentDate:           documentDate,
			DocumentNumber:         in.DocumentNumber,
			ExternalDocumentNumber: in.ExternalDocumentNumber,
			InternalDocumentNumber: in.InternalDocumentNumber,
			DocumentText:           in.DocumentText,
			Account:                row.Account,
			SummaryAccount:         row.SummaryAccount,
			Amount:                 row.Amount,
			RowText:                row.RowText,
			VatKey:                 row.VatKey,
			Dimensions:             row.Dimensions,
			ForeignCurrencyAmount:  row.ForeignCurrencyAmount,
			ForeignCurrencyCode:    row.ForeignCurrencyCode,
			ForeignCurrencyRate:    row.ForeignCurrencyRate,
			DiscountPercent1:       row.DiscountPercent1,
			DiscountPeriod1:        row.DiscountPeriod1,
			DiscountPercent2:       row.DiscountPercent2,
			DiscountPeriod2:        row.DiscountPeriod2,
			NetTimeLimit:           row.NetTimeLimit,
			DiscountAccount:        row.DiscountAccount,
			PaymentType:            row.PaymentType,
			UstID:                  row.UstID,
		})
	}
	return req, nil
}

// CanonicalRow is the comparison-normal form of one posting row.
type CanonicalRow struct {
	Account    string            `json:"account"`
	Amount     string            `json:"amount"`
	VatKey     string            `json:"vatKey,omitempty"`
	RowText    string            `json:"rowText,omitempty"`
	Dimensions map[string]string `json:"dimensions,omitempty"`
}

// CanonicalBuchung is the comparison-normal form of a whole Buchung. Rows are
// sorted by (Account, Amount, VatKey, RowText).
type CanonicalBuchung struct {
	DocumentNumber string         `json:"documentNumber"`
	PostingDate    string         `json:"postingDate"`
	DocumentText   string         `json:"documentText,omitempty"`
	Rows           []CanonicalRow `json:"rows"`
}

func (in SinglePostingInput) Canonical() CanonicalBuchung {
	rows := make([]CanonicalRow, 0, len(in.Rows))
	for _, row := range in.Rows {
		canonical := CanonicalRow{
			Account: row.Account,
			Amount:  row.Amount.String(),
			VatKey:  row.VatKey,
			RowText: row.RowText,
		}
		if len(row.Dimensions) > 0 {
			canonical.Dimensions = map[string]string{}
			for _, dim := range row.Dimensions {
				canonical.Dimensions[dim.DimensionID] = dim.DimensionAccountNumber
			}
		}
		rows = append(rows, canonical)
	}
	sortCanonicalRows(rows)
	return CanonicalBuchung{
		DocumentNumber: in.DocumentNumber,
		PostingDate:    in.PostingDate,
		DocumentText:   in.DocumentText,
		Rows:           rows,
	}
}

func sortCanonicalRows(rows []CanonicalRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Account != b.Account {
			return a.Account < b.Account
		}
		if a.Amount != b.Amount {
			return a.Amount < b.Amount
		}
		if a.VatKey != b.VatKey {
			return a.VatKey < b.VatKey
		}
		return a.RowText < b.RowText
	})
}

// CanonicalFromJournal reduces GET /journal/{documentNumber} records to the
// canonical comparison form.
func CanonicalFromJournal(records []any) (CanonicalBuchung, error) {
	if len(records) == 0 {
		return CanonicalBuchung{}, errors.New("journal contains no records")
	}
	out := CanonicalBuchung{Rows: []CanonicalRow{}}
	for i, item := range records {
		rec, ok := item.(map[string]any)
		if !ok {
			return CanonicalBuchung{}, fmt.Errorf("journal record %d is not an object", i)
		}
		debit, hasDebit := rec["debitAmount"].(float64)
		credit, hasCredit := rec["creditAmount"].(float64)
		amount, hasAmount := rec["amount"].(float64)
		var value Amount
		switch {
		case hasDebit || hasCredit:
			value = AmountFromFloat(debit - credit)
		case hasAmount:
			value = AmountFromFloat(amount)
		}
		row := CanonicalRow{
			Amount:  value.String(),
			VatKey:  stringField(rec, "vatKey"),
			RowText: stringField(rec, "postingText"),
		}
		if v := rec["accountNumber"]; v != nil {
			row.Account = fmt.Sprint(v)
		}
		for n := 1; n <= 10; n++ {
			key := fmt.Sprintf("documentDimension_%d", n)
			if v := rec[key]; v != nil {
				if row.Dimensions == nil {
					row.Dimensions = map[string]string{}
				}
				row.Dimensions[fmt.Sprintf("dimension_%d", n)] = fmt.Sprint(v)
			}
		}
		out.Rows = append(out.Rows, row)
		if i == 0 {
			out.DocumentNumber = stringField(rec, "documentNumber")
			out.PostingDate = journalPostingDate(rec["postingDate"])
			out.DocumentText = stringField(rec, "documentText")
		}
	}
	sortCanonicalRows(out.Rows)
	return out, nil
}

func stringField(rec map[string]any, key string) string {
	if s, ok := rec[key].(string); ok {
		return s
	}
	return ""
}

func journalPostingDate(value any) string {
	switch v := value.(type) {
	case float64:
		return time.UnixMilli(int64(v)).UTC().Format("2006-01-02")
	case string:
		if len(v) >= 10 && v[4] == '-' && v[7] == '-' {
			return v[:10]
		}
		if t, err := parseScopevisioDate(v); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// BuchungDiff is the result of comparing a requested Buchung against the
// journal's canonical form.
type BuchungDiff struct {
	Missing    []CanonicalRow `json:"missing,omitempty"`
	Unexpected []CanonicalRow `json:"unexpected,omitempty"`
	Generated  []CanonicalRow `json:"generated,omitempty"`
	Fields     []string       `json:"fields,omitempty"`
}

func (d BuchungDiff) Equal() bool {
	return len(d.Missing) == 0 && len(d.Unexpected) == 0 && len(d.Fields) == 0
}

// CompareBuchung multiset-matches expected against actual rows. Rows present
// only in actual land in Generated when allowGenerated is set (the API may add
// tax rows under autoCreateTax), otherwise in Unexpected.
func CompareBuchung(expected, actual CanonicalBuchung, allowGenerated bool) BuchungDiff {
	diff := BuchungDiff{}
	used := make([]bool, len(actual.Rows))
	for _, want := range expected.Rows {
		matched := false
		for j, got := range actual.Rows {
			if used[j] || !canonicalRowEqual(want, got) {
				continue
			}
			used[j] = true
			matched = true
			break
		}
		if !matched {
			diff.Missing = append(diff.Missing, want)
		}
	}
	for j, got := range actual.Rows {
		if used[j] {
			continue
		}
		if allowGenerated {
			diff.Generated = append(diff.Generated, got)
		} else {
			diff.Unexpected = append(diff.Unexpected, got)
		}
	}
	if expected.DocumentNumber != actual.DocumentNumber {
		diff.Fields = append(diff.Fields, "documentNumber")
	}
	if expected.PostingDate != actual.PostingDate {
		diff.Fields = append(diff.Fields, "postingDate")
	}
	if expected.DocumentText != "" && expected.DocumentText != actual.DocumentText {
		diff.Fields = append(diff.Fields, "documentText")
	}
	return diff
}

func canonicalRowEqual(a, b CanonicalRow) bool {
	if a.Account != b.Account || a.Amount != b.Amount || a.VatKey != b.VatKey || a.RowText != b.RowText {
		return false
	}
	if len(a.Dimensions) != len(b.Dimensions) {
		return false
	}
	for key, value := range a.Dimensions {
		if b.Dimensions[key] != value {
			return false
		}
	}
	return true
}
