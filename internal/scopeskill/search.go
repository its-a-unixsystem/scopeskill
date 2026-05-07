package scopeskill

import (
	"errors"
	"fmt"
)

const (
	DefaultSearchPageSize = 100
	MaxSearchPageSize     = 1000
)

type SearchOperator string

const (
	OpStartsWith SearchOperator = "startswith"
	OpEndsWith   SearchOperator = "endswith"
	OpContains   SearchOperator = "contains"
	OpEquals     SearchOperator = "equals"
	OpNotEquals  SearchOperator = "notequals"
)

func (op SearchOperator) Valid() bool {
	switch op {
	case OpStartsWith, OpEndsWith, OpContains, OpEquals, OpNotEquals:
		return true
	}
	return false
}

type SearchCondition struct {
	Field    string
	Operator SearchOperator
	Value    any
}

type SearchRequest struct {
	Page       int
	PageSize   int
	Fields     []string
	Conditions []SearchCondition
	Order      []string
}

func (r SearchRequest) Body() (map[string]any, error) {
	pageSize := r.PageSize
	if pageSize <= 0 {
		pageSize = DefaultSearchPageSize
	}
	if pageSize > MaxSearchPageSize {
		pageSize = MaxSearchPageSize
	}
	body := map[string]any{
		"page":     r.Page,
		"pageSize": pageSize,
	}
	if len(r.Fields) > 0 {
		body["fields"] = append([]string{}, r.Fields...)
	}
	if len(r.Conditions) > 0 {
		search := make([]map[string]any, 0, len(r.Conditions))
		for _, c := range r.Conditions {
			if c.Field == "" {
				return nil, errors.New("search condition is missing field")
			}
			if !c.Operator.Valid() {
				return nil, fmt.Errorf("unknown search operator: %q", c.Operator)
			}
			search = append(search, map[string]any{
				"field":    c.Field,
				"operator": string(c.Operator),
				"value":    c.Value,
			})
		}
		body["search"] = search
	}
	if len(r.Order) > 0 {
		body["order"] = append([]string{}, r.Order...)
	}
	return body, nil
}

// RecordsFromResponse extracts the record list from a Scopevisio search
// response. Some endpoints return a top-level array; others wrap the array
// under "results" or "data".
func RecordsFromResponse(raw any) ([]any, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []any:
		return v, nil
	case map[string]any:
		if r, ok := v["results"].([]any); ok {
			return r, nil
		}
		if r, ok := v["data"].([]any); ok {
			return r, nil
		}
	}
	return nil, errors.New("unexpected search response shape: expected JSON array, or object with results/data array")
}
