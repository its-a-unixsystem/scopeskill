package scopeskill

import (
	"errors"
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
