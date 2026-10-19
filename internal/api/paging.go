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
