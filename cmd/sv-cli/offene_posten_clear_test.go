package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCreditorClearingInput(t *testing.T) {
	t.Parallel()

	valid := `{
		"paymentDocumentNumber":"PAY-2026-7",
		"items":[
			{"documentNumber":"INV-1","clearingAmount":119.00},
			{"documentNumber":"INV-2","clearingAmount":0.01}
		]
	}`
	input, err := parseCreditorClearingInput([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if input.PaymentDocumentNumber != "PAY-2026-7" || len(input.Items) != 2 {
		t.Fatalf("input = %#v", input)
	}
	request := input.providerRequest()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"clearings":[{"documentNumber":"PAY-2026-7","documents":[{"documentNumber":"INV-1","clearingAmount":119.00},{"documentNumber":"INV-2","clearingAmount":0.01}]}]}`
	if string(raw) != want {
		t.Fatalf("provider request = %s, want %s", raw, want)
	}
}

func TestParseCreditorClearingInputRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"unknown field", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":1}],"extra":true}`, "unknown field"},
		{"missing payment", `{"items":[{"documentNumber":"INV","clearingAmount":1}]}`, "paymentDocumentNumber"},
		{"missing items", `{"paymentDocumentNumber":"PAY","items":[]}`, "items"},
		{"missing item number", `{"paymentDocumentNumber":"PAY","items":[{"clearingAmount":1}]}`, "documentNumber"},
		{"duplicate item", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":1},{"documentNumber":"INV","clearingAmount":2}]}`, "duplicate"},
		{"payment repeated as item", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"PAY","clearingAmount":1}]}`, "payment document"},
		{"zero", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":0}]}`, "positive"},
		{"negative", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":-1}]}`, "positive"},
		{"fractional cent", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":1.001}]}`, "two decimal"},
		{"string amount", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":"1.00"}]}`, "number"},
		{"incomplete before-state", `{"paymentDocumentNumber":"PAY","paymentOpenAmount":2,"items":[{"documentNumber":"INV","clearingAmount":1}]}`, "supplied together"},
		{"trailing JSON", `{"paymentDocumentNumber":"PAY","items":[{"documentNumber":"INV","clearingAmount":1}]} {}`, "trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseCreditorClearingInput([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
