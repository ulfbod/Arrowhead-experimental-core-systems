package model

import "sort"

// PageRequest carries AH5 pagination parameters.
// A zero value (PageSize == 0) means "return the full collection".
type PageRequest struct {
	PageNumber    int    `json:"pageNumber"`
	PageSize      int    `json:"pageSize"`
	PageSortField string `json:"pageSortField"`
	PageDirection string `json:"pageDirection"` // "ASC" | "DESC"
}

// Paginate sorts items by sortKey and returns the requested page together with
// the total pre-pagination count. A zero PageRequest returns the full collection.
func Paginate[T any](items []T, req PageRequest, sortKey func(T) string) ([]T, int) {
	total := len(items)
	if total == 0 {
		return items, 0
	}
	desc := req.PageDirection == "DESC"
	sort.SliceStable(items, func(i, j int) bool {
		ki, kj := sortKey(items[i]), sortKey(items[j])
		if desc {
			return ki > kj
		}
		return ki < kj
	})
	if req.PageSize <= 0 {
		return items, total
	}
	start := req.PageNumber * req.PageSize
	if start >= total {
		return []T{}, total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	return items[start:end], total
}
