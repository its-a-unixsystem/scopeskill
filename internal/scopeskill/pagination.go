package scopeskill

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

const DefaultMaxResults = 10000

// PageFetcher executes one paginated request and returns the records on that
// page. The body is the request body produced by SearchRequest.Body().
type PageFetcher func(body map[string]any) ([]any, error)

type PaginateOptions struct {
	All         bool
	PageSize    int
	Max         int
	CursorField string
}

// Paginate runs the search loop. With All=false a single request is issued at
// PageSize (default DefaultSearchPageSize). With All=true the loop pages at
// MaxSearchPageSize, terminates on a short page, and stops once Max records
// have been gathered (default DefaultMaxResults).
// CursorField opts bulk reads into ascending identity traversal, rejecting
// missing, invalid, repeated, or backward identities without partial results.
func Paginate(opts PaginateOptions, base SearchRequest, fetch PageFetcher) ([]any, error) {
	if !opts.All {
		req := base
		req.Page = 0
		if opts.PageSize > 0 {
			req.PageSize = opts.PageSize
		} else {
			req.PageSize = DefaultSearchPageSize
		}
		body, err := req.Body()
		if err != nil {
			return nil, err
		}
		return fetch(body)
	}

	max := opts.Max
	if max <= 0 {
		max = DefaultMaxResults
	}

	out := []any{}
	page := 0
	var previous int64
	if opts.CursorField != "" && len(base.Fields) > 0 && !slices.Contains(base.Fields, opts.CursorField) {
		base.Fields = append(slices.Clone(base.Fields), opts.CursorField)
	}
	for {
		req := base
		req.Page = page
		req.PageSize = MaxSearchPageSize
		if opts.CursorField != "" {
			req.Page = 0
			req.Order = nil
		}
		body, err := req.Body()
		if err != nil {
			return nil, err
		}
		if opts.CursorField != "" {
			body["afterId"] = previous
		}
		records, err := fetch(body)
		if err != nil {
			return nil, err
		}
		if opts.CursorField != "" {
			for i, record := range records {
				row, ok := record.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("unsafe pagination: missing or invalid %s at record %d", opts.CursorField, len(out)+i+1)
				}
				id, err := paginationCursorID(row[opts.CursorField])
				if err != nil {
					return nil, fmt.Errorf("unsafe pagination: missing or invalid %s at record %d", opts.CursorField, len(out)+i+1)
				}
				if id <= previous {
					return nil, fmt.Errorf("unsafe pagination: %s must increase (previous=%d, current=%d)", opts.CursorField, previous, id)
				}
				previous = id
			}
		}
		out = append(out, records...)
		if len(out) >= max {
			return out[:max], nil
		}
		if len(records) < MaxSearchPageSize {
			return out, nil
		}
		if opts.CursorField == "" {
			page++
		}
	}
}

func paginationCursorID(value any) (int64, error) {
	var id int64
	switch v := value.(type) {
	case int:
		id = int64(v)
	case int64:
		id = v
	case json.Number:
		var err error
		id, err = v.Int64()
		if err != nil {
			return 0, err
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v > 9007199254740991 || v != math.Trunc(v) || v <= 0 {
			return 0, fmt.Errorf("invalid cursor identity")
		}
		id = int64(v)
	default:
		return 0, fmt.Errorf("invalid cursor identity")
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid cursor identity")
	}
	return id, nil
}
