package scopeskill

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
)

type fetchCall struct {
	page     int
	pageSize int
}

// makeFetcher returns a fetcher driven by a list of page payloads. Each call
// pops the next payload; surplus calls fail the test by returning an error.
func makeFetcher(t *testing.T, pages [][]any, calls *[]fetchCall) PageFetcher {
	t.Helper()
	idx := 0
	return func(body map[string]any) ([]any, error) {
		page, _ := body["page"].(int)
		size, _ := body["pageSize"].(int)
		*calls = append(*calls, fetchCall{page: page, pageSize: size})
		if idx >= len(pages) {
			return nil, errors.New("fetcher called more times than expected")
		}
		out := pages[idx]
		idx++
		return out, nil
	}
}

func makeRecords(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestPaginateSinglePageDefault(t *testing.T) {
	var calls []fetchCall
	fetch := makeFetcher(t, [][]any{makeRecords(7)}, &calls)
	got, err := Paginate(PaginateOptions{}, SearchRequest{}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 {
		t.Fatalf("len = %d", len(got))
	}
	if len(calls) != 1 || calls[0].page != 0 || calls[0].pageSize != 100 {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestPaginateSinglePageRespectsCustomPageSize(t *testing.T) {
	var calls []fetchCall
	fetch := makeFetcher(t, [][]any{makeRecords(3)}, &calls)
	if _, err := Paginate(PaginateOptions{PageSize: 25}, SearchRequest{}, fetch); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].pageSize != 25 {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestPaginateAllShortLastPageTerminates(t *testing.T) {
	var calls []fetchCall
	pages := [][]any{makeRecords(MaxSearchPageSize), makeRecords(50)}
	fetch := makeFetcher(t, pages, &calls)
	got, err := Paginate(PaginateOptions{All: true}, SearchRequest{}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxSearchPageSize+50 {
		t.Fatalf("len = %d", len(got))
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	for i, c := range calls {
		if c.pageSize != MaxSearchPageSize {
			t.Fatalf("call %d pageSize = %d", i, c.pageSize)
		}
		if c.page != i {
			t.Fatalf("call %d page = %d", i, c.page)
		}
	}
}

func TestPaginateAllExactMultipleBoundaryTerminatesOnEmptyPage(t *testing.T) {
	var calls []fetchCall
	pages := [][]any{makeRecords(MaxSearchPageSize), makeRecords(MaxSearchPageSize), makeRecords(0)}
	fetch := makeFetcher(t, pages, &calls)
	got, err := Paginate(PaginateOptions{All: true, Max: 10000}, SearchRequest{}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2*MaxSearchPageSize {
		t.Fatalf("len = %d", len(got))
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestPaginateAllSafetyCapTruncates(t *testing.T) {
	var calls []fetchCall
	full := makeRecords(MaxSearchPageSize)
	pages := [][]any{full, full, full, full, full, full, full, full, full, full, full, full}
	fetch := makeFetcher(t, pages, &calls)
	got, err := Paginate(PaginateOptions{All: true}, SearchRequest{}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != DefaultMaxResults {
		t.Fatalf("len = %d, want %d", len(got), DefaultMaxResults)
	}
	if len(calls) != 10 {
		t.Fatalf("expected to stop after 10 full pages, calls = %+v", calls)
	}
}

func TestPaginateAllRespectsExplicitMax(t *testing.T) {
	var calls []fetchCall
	pages := [][]any{makeRecords(MaxSearchPageSize), makeRecords(MaxSearchPageSize), makeRecords(50)}
	fetch := makeFetcher(t, pages, &calls)
	got, err := Paginate(PaginateOptions{All: true, Max: 1500}, SearchRequest{}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1500 {
		t.Fatalf("len = %d", len(got))
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestPaginateAllPropagatesMidLoopError(t *testing.T) {
	calls := 0
	boom := errors.New("network blip")
	fetch := PageFetcher(func(body map[string]any) ([]any, error) {
		calls++
		if calls == 1 {
			return makeRecords(MaxSearchPageSize), nil
		}
		return nil, boom
	})
	if _, err := Paginate(PaginateOptions{All: true}, SearchRequest{}, fetch); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestPaginateForwardsBaseRequestFieldsAndConditions(t *testing.T) {
	var seen map[string]any
	fetch := PageFetcher(func(body map[string]any) ([]any, error) {
		seen = body
		return []any{}, nil
	})
	base := SearchRequest{
		Fields:     []string{"id", "number"},
		Conditions: []SearchCondition{{Field: "name", Operator: OpContains, Value: "x"}},
		Order:      []string{"number = asc"},
	}
	if _, err := Paginate(PaginateOptions{}, base, fetch); err != nil {
		t.Fatal(err)
	}
	wantSearch := []map[string]any{{"field": "name", "operator": "contains", "value": "x"}}
	if !reflect.DeepEqual(seen["search"], wantSearch) {
		t.Fatalf("search = %#v", seen["search"])
	}
	if !reflect.DeepEqual(seen["fields"], []string{"id", "number"}) {
		t.Fatalf("fields = %#v", seen["fields"])
	}
	if !reflect.DeepEqual(seen["order"], []string{"number = asc"}) {
		t.Fatalf("order = %#v", seen["order"])
	}
}

func cursorRecords(first, count int) []any {
	rows := make([]any, count)
	for i := range rows {
		rows[i] = map[string]any{"rowNumber": first + i, "documentNumber": "same"}
	}
	return rows
}

func assertCursorSequence(t *testing.T, rows []any, first, count int) {
	t.Helper()
	if len(rows) != count {
		t.Fatalf("got %d rows, want %d", len(rows), count)
	}
	for i, row := range rows {
		id, err := paginationCursorID(row.(map[string]any)["rowNumber"])
		if err != nil || id != int64(first+i) {
			t.Fatalf("row %d identity = %v, err = %v", i, id, err)
		}
	}
}

func TestPaginateCursorTraversal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		count, max int
	}{
		{"empty", 0, 0},
		{"equal business projections", 1003, 0},
		{"exact full page", 1000, 0},
		{"explicit cap", 2003, 1001},
		{"default cap", 11000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber", Max: tc.max}, SearchRequest{}, func(body map[string]any) ([]any, error) {
				calls++
				after := int(body["afterId"].(int64))
				count := min(MaxSearchPageSize, tc.count-after)
				return cursorRecords(after+1, count), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			cap := tc.max
			if cap == 0 {
				cap = DefaultMaxResults
			}
			assertCursorSequence(t, got, 1, min(tc.count, cap))
			wantCalls := (min(tc.count, cap) + 999) / 1000
			if tc.count < cap && tc.count%1000 == 0 {
				wantCalls++
			}
			if calls != wantCalls {
				t.Fatalf("calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestPaginateCursorRejectsInvalidIdentity(t *testing.T) {
	for _, value := range []any{nil, "1", true, 0, -1, int64(-1), 1.5, math.NaN(), math.Inf(1), math.Inf(-1), float64(9007199254740992), json.Number("1.2"), json.Number("9223372036854775808")} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, SearchRequest{}, func(map[string]any) ([]any, error) {
				return []any{map[string]any{"rowNumber": value}}, nil
			})
			if got != nil || err == nil || err.Error() != "unsafe pagination: missing or invalid rowNumber at record 1" {
				t.Fatalf("got = %v, err = %v", got, err)
			}
		})
	}
	for _, row := range []any{nil, 5, map[string]any{}} {
		got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, SearchRequest{}, func(map[string]any) ([]any, error) { return []any{row}, nil })
		if got != nil || err == nil {
			t.Fatalf("got = %v, err = %v", got, err)
		}
	}
}

func TestPaginateCursorAcceptsIntegerRepresentations(t *testing.T) {
	values := []any{int(1), int64(2), json.Number("3"), float64(4), float64(9007199254740991), json.Number("9223372036854775807")}
	rows := make([]any, len(values))
	for i, value := range values {
		rows[i] = map[string]any{"rowNumber": value}
	}
	got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, SearchRequest{}, func(map[string]any) ([]any, error) { return rows, nil })
	if err != nil || !reflect.DeepEqual(got, rows) {
		t.Fatalf("got = %v, err = %v", got, err)
	}
}

func TestPaginateCursorRejectsNonIncreasingIdentity(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pages [][]any
		max   int
		want  string
	}{
		{"within duplicate", [][]any{append(cursorRecords(1, 2), cursorRecords(2, 1)...)}, 0, "previous=2, current=2"},
		{"within backward", [][]any{append(cursorRecords(1, 2), cursorRecords(1, 1)...)}, 0, "previous=2, current=1"},
		{"across duplicate", [][]any{cursorRecords(1, 1000), cursorRecords(1000, 1)}, 0, "previous=1000, current=1000"},
		{"across backward", [][]any{cursorRecords(1, 1000), cursorRecords(999, 1)}, 0, "previous=1000, current=999"},
		{"ignored cursor", [][]any{cursorRecords(1, 1000), cursorRecords(1, 1000)}, 0, "previous=1000, current=1"},
		{"invalid beyond cap", [][]any{append(cursorRecords(1, 2), cursorRecords(1, 1)...)}, 1, "previous=2, current=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []fetchCall
			got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber", Max: tc.max}, SearchRequest{}, makeFetcher(t, tc.pages, &calls))
			want := "unsafe pagination: rowNumber must increase (" + tc.want + ")"
			if got != nil || err == nil || err.Error() != want {
				t.Fatalf("got = %v, err = %v, want %s", got, err, want)
			}
		})
	}
}

func TestPaginateCursorMidPullError(t *testing.T) {
	boom := errors.New("fetch failed")
	calls := 0
	got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, SearchRequest{}, func(map[string]any) ([]any, error) {
		calls++
		if calls == 1 {
			return cursorRecords(1, 1000), nil
		}
		return nil, boom
	})
	if got != nil || !errors.Is(err, boom) {
		t.Fatalf("got = %v, err = %v", got, err)
	}
}

func TestPaginateCursorInsertionDoesNotSkipSuffix(t *testing.T) {
	rows := cursorRecords(2, 1003)
	calls := 0
	got, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, SearchRequest{}, func(body map[string]any) ([]any, error) {
		calls++
		if calls == 2 {
			rows = append(cursorRecords(1, 1), rows...)
		}
		after := body["afterId"].(int64)
		page := []any{}
		for _, row := range rows {
			id, _ := paginationCursorID(row.(map[string]any)["rowNumber"])
			if id > after && len(page) < MaxSearchPageSize {
				page = append(page, row)
			}
		}
		return page, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertCursorSequence(t, got, 2, 1003)
}

func TestPaginateCursorProjectionAndRequestIsolation(t *testing.T) {
	for _, fields := range [][]string{nil, {"documentNumber"}, {"documentNumber", "rowNumber"}} {
		base := SearchRequest{Fields: fields, Order: []string{"documentNumber = asc"}, Conditions: []SearchCondition{{Field: "name", Operator: OpEquals, Value: "x"}}}
		original := append([]string(nil), fields...)
		_, err := Paginate(PaginateOptions{All: true, CursorField: "rowNumber"}, base, func(body map[string]any) ([]any, error) {
			if body["afterId"] != int64(0) || body["page"] != 0 || body["order"] != nil {
				t.Fatalf("unsafe cursor request: %#v", body)
			}
			want := original
			if len(fields) == 1 {
				want = []string{"documentNumber", "rowNumber"}
			}
			if len(want) == 0 {
				if _, exists := body["fields"]; exists {
					t.Fatalf("full projection narrowed: %#v", body)
				}
			} else if !reflect.DeepEqual(body["fields"], want) {
				t.Fatalf("fields = %#v", body["fields"])
			}
			if !reflect.DeepEqual(body["search"], []map[string]any{{"field": "name", "operator": "equals", "value": "x"}}) {
				t.Fatalf("filter lost: %#v", body)
			}
			return nil, nil
		})
		if err != nil || !reflect.DeepEqual(base.Fields, original) || !reflect.DeepEqual(base.Order, []string{"documentNumber = asc"}) {
			t.Fatalf("base mutated: %#v, err = %v", base, err)
		}
	}
}

func TestPaginateCursorSinglePagePreservesBehavior(t *testing.T) {
	want := []any{map[string]any{"documentNumber": "no identity required"}}
	got, err := Paginate(PaginateOptions{CursorField: "rowNumber"}, SearchRequest{Order: []string{"postingDate = asc"}}, func(body map[string]any) ([]any, error) {
		if _, exists := body["afterId"]; exists {
			t.Fatal("single-page request uses cursor")
		}
		if !reflect.DeepEqual(body["order"], []string{"postingDate = asc"}) {
			t.Fatal("single-page order changed")
		}
		return want, nil
	})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got = %v, err = %v", got, err)
	}
}
