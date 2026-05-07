package scopeskill

const DefaultMaxResults = 10000

// PageFetcher executes one paginated request and returns the records on that
// page. The body is the request body produced by SearchRequest.Body().
type PageFetcher func(body map[string]any) ([]any, error)

type PaginateOptions struct {
	All      bool
	PageSize int
	Max      int
}

// Paginate runs the search loop. With All=false a single request is issued at
// PageSize (default DefaultSearchPageSize). With All=true the loop pages at
// MaxSearchPageSize, terminates on a short page, and stops once Max records
// have been gathered (default DefaultMaxResults).
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
	for {
		req := base
		req.Page = page
		req.PageSize = MaxSearchPageSize
		body, err := req.Body()
		if err != nil {
			return nil, err
		}
		records, err := fetch(body)
		if err != nil {
			return nil, err
		}
		out = append(out, records...)
		if len(out) >= max {
			return out[:max], nil
		}
		if len(records) < MaxSearchPageSize {
			return out, nil
		}
		page++
	}
}
