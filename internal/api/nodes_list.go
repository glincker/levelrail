package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const nodeListMaxLimit = 500

// nodeFilterStore is the optional filtered-list surface; *store.DB has
// it. Mirrors appFilterStore (apps_list.go): NodeStore keeps its plain
// unfiltered ListNodes, so every existing caller of that interface is
// untouched by this.
type nodeFilterStore interface {
	ListNodesFiltered(ctx context.Context, f store.NodeListFilter) ([]store.Node, int, error)
}

// parseNodeListFilter reads q, limit and offset off r, the same
// parameter names and shape parseAppListFilter uses.
func parseNodeListFilter(r *http.Request) store.NodeListFilter {
	q := r.URL.Query()
	f := store.NodeListFilter{Query: strings.TrimSpace(q.Get("q"))}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		f.Limit = min(n, nodeListMaxLimit)
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		f.Offset = n
	}
	return f
}
