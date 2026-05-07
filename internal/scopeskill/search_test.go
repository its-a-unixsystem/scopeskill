package scopeskill

import (
	"reflect"
	"testing"
)

func TestSearchRequestBody(t *testing.T) {
	tests := []struct {
		name string
		req  SearchRequest
		want map[string]any
	}{
		{
			name: "empty request uses default pageSize",
			req:  SearchRequest{},
			want: map[string]any{"page": 0, "pageSize": 100},
		},
		{
			name: "explicit pageSize is preserved",
			req:  SearchRequest{PageSize: 250},
			want: map[string]any{"page": 0, "pageSize": 250},
		},
		{
			name: "pageSize cap is enforced",
			req:  SearchRequest{PageSize: 5000},
			want: map[string]any{"page": 0, "pageSize": 1000},
		},
		{
			name: "fields, order and conditions are emitted",
			req: SearchRequest{
				Page:     2,
				PageSize: 50,
				Fields:   []string{"id", "number", "name"},
				Conditions: []SearchCondition{
					{Field: "name", Operator: OpContains, Value: "Reise"},
					{Field: "number", Operator: OpStartsWith, Value: "4"},
				},
				Order: []string{"number = asc"},
			},
			want: map[string]any{
				"page":     2,
				"pageSize": 50,
				"fields":   []string{"id", "number", "name"},
				"search": []map[string]any{
					{"field": "name", "operator": "contains", "value": "Reise"},
					{"field": "number", "operator": "startswith", "value": "4"},
				},
				"order": []string{"number = asc"},
			},
		},
		{
			name: "every operator round-trips as no-space camelCase",
			req: SearchRequest{
				Conditions: []SearchCondition{
					{Field: "a", Operator: OpStartsWith, Value: "1"},
					{Field: "b", Operator: OpEndsWith, Value: "2"},
					{Field: "c", Operator: OpContains, Value: "3"},
					{Field: "d", Operator: OpEquals, Value: "4"},
					{Field: "e", Operator: OpNotEquals, Value: "5"},
				},
			},
			want: map[string]any{
				"page":     0,
				"pageSize": 100,
				"search": []map[string]any{
					{"field": "a", "operator": "startswith", "value": "1"},
					{"field": "b", "operator": "endswith", "value": "2"},
					{"field": "c", "operator": "contains", "value": "3"},
					{"field": "d", "operator": "equals", "value": "4"},
					{"field": "e", "operator": "notequals", "value": "5"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.Body()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Body() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSearchRequestBodyRejectsInvalidConditions(t *testing.T) {
	tests := []struct {
		name string
		req  SearchRequest
	}{
		{
			name: "missing field",
			req:  SearchRequest{Conditions: []SearchCondition{{Operator: OpEquals, Value: 1}}},
		},
		{
			name: "unknown operator",
			req: SearchRequest{Conditions: []SearchCondition{
				{Field: "name", Operator: SearchOperator("starts with"), Value: "x"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.req.Body(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRecordsFromResponse(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		want    []any
		wantErr bool
	}{
		{name: "nil", raw: nil, want: nil},
		{name: "top-level array", raw: []any{map[string]any{"id": 1}}, want: []any{map[string]any{"id": 1}}},
		{name: "wrapped under results", raw: map[string]any{"results": []any{1.0, 2.0}}, want: []any{1.0, 2.0}},
		{name: "wrapped under data", raw: map[string]any{"data": []any{"a"}}, want: []any{"a"}},
		{name: "string is not a list", raw: "nope", wantErr: true},
		{name: "object without recognised key", raw: map[string]any{"foo": []any{1.0}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RecordsFromResponse(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("RecordsFromResponse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
