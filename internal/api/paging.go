package api

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
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

// pageQueryDefault is pageQuery with a default page size for callers that sent no limit,
// so an unbounded table never serializes whole into one response.
func pageQueryDefault[T any](w http.ResponseWriter, r *http.Request, items []T, def int) []T {
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err != nil || n <= 0 {
		q := r.URL.Query()
		q.Set("limit", strconv.Itoa(def))
		r = r.Clone(r.Context())
		r.URL.RawQuery = q.Encode()
	}
	return pageQuery(w, r, items)
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

// sortQuery orders items by the ?sort= key (a leading "-" reverses it). Unknown keys keep the
// store order so a stale client never gets an error for a column it invented.
func sortQuery[T any](r *http.Request, items []T, cmps map[string]func(a, b T) int) {
	key := r.URL.Query().Get("sort")
	desc := strings.HasPrefix(key, "-")
	cmp, ok := cmps[strings.TrimPrefix(key, "-")]
	if !ok {
		return
	}
	slices.SortStableFunc(items, func(a, b T) int {
		if desc {
			return cmp(b, a)
		}
		return cmp(a, b)
	})
}
