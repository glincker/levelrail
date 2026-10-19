package api

import (
	"net/http"
	"strconv"
)

// pageQuery applies optional limit (capped at appListMaxLimit) and offset query params to items
// and sets X-Total-Count to the unpaged length. Without limit the full list is returned.
func pageQuery[T any](w http.ResponseWriter, r *http.Request, items []T) []T {
	w.Header().Set("X-Total-Count", strconv.Itoa(len(items)))
	q := r.URL.Query()
	offset := 0
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		offset = min(n, len(items))
	}
	items = items[offset:]
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		items = items[:min(n, appListMaxLimit, len(items))]
	}
	return items
}

// pageInfo is the envelope block struct-shaped list responses carry when the
// caller sent limit or offset. NextOffset is omitted on the last page.
type pageInfo struct {
	Total      int  `json:"total"`
	Limit      int  `json:"limit"`
	Offset     int  `json:"offset"`
	NextOffset *int `json:"next_offset,omitempty"`
}

// pageWithInfo pages items like pageQuery and also returns the envelope block,
// nil when neither limit nor offset was requested so existing callers see no change.
func pageWithInfo[T any](w http.ResponseWriter, r *http.Request, items []T) ([]T, *pageInfo) {
	q := r.URL.Query()
	total := len(items)
	paged := pageQuery(w, r, items)
	if q.Get("limit") == "" && q.Get("offset") == "" {
		return paged, nil
	}
	offset := 0
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		offset = min(n, total)
	}
	info := &pageInfo{Total: total, Limit: len(paged), Offset: offset}
	if next := offset + len(paged); next < total {
		info.NextOffset = &next
	}
	return paged, info
}
