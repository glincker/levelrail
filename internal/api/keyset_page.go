package api

import (
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// nextCursorHeader carries the cursor for the next page of a list that
// returns a bare JSON array, so existing clients keep their response shape.
const nextCursorHeader = "X-Next-Cursor"

// parseKeysetCursor reads ?cursor, writing a 400 and returning false when it
// is present but was not minted by a keyset list.
func parseKeysetCursor(w http.ResponseWriter, r *http.Request) (string, bool) {
	c := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if c == "" {
		return "", true
	}
	if _, _, err := store.DecodeKeysetCursor(c); err != nil {
		writeError(w, http.StatusBadRequest, "cursor is not valid")
		return "", false
	}
	return c, true
}
