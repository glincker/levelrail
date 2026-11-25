package api

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

// The store implements these; fakes that do not are simply never blocked.
type databaseDeletableChecker interface {
	CheckDatabaseDeletable(ctx context.Context, name string) error
}

type databaseBranchLister interface {
	ListAllDatabaseBranches(ctx context.Context) ([]store.DatabaseBranch, error)
}

func isBranchBlock(err error) bool {
	return errors.Is(err, store.ErrDatabaseBranchProtected) || errors.Is(err, store.ErrDatabaseBranchHasChild)
}

func branchBlockMessage(err error) string {
	if errors.Is(err, store.ErrDatabaseBranchProtected) {
		return "database is a protected branch; remove its protection first"
	}
	return "database has branches; delete or promote them first"
}

// checkDatabaseDeletable runs before any container or volume is touched.
func (rt *Router) checkDatabaseDeletable(ctx context.Context, name string) error {
	if c, ok := rt.databases.(databaseDeletableChecker); ok {
		return c.CheckDatabaseDeletable(ctx, name)
	}
	return nil
}

// writeBranchBlock answers 409 for a branch refusal and reports whether it did.
func writeBranchBlock(w http.ResponseWriter, err error) bool {
	if !isBranchBlock(err) {
		return false
	}
	writeError(w, http.StatusConflict, branchBlockMessage(err))
	return true
}

// childrenFirst orders databases so a branch is deleted before its parent.
func (rt *Router) childrenFirst(ctx context.Context, dbs []store.DesiredDatabase) []store.DesiredDatabase {
	l, ok := rt.databases.(databaseBranchLister)
	if !ok {
		return dbs
	}
	rows, err := l.ListAllDatabaseBranches(ctx)
	if err != nil || len(rows) == 0 {
		return dbs
	}
	parent := make(map[string]string, len(rows))
	for _, b := range rows {
		parent[b.DatabaseName] = b.ParentName
	}
	depth := func(name string) int {
		d := 0
		for i := 0; i < 64; i++ {
			p, ok := parent[name]
			if !ok || p == "" {
				break
			}
			d++
			name = p
		}
		return d
	}
	out := append([]store.DesiredDatabase(nil), dbs...)
	sort.SliceStable(out, func(i, j int) bool { return depth(out[i].Name) > depth(out[j].Name) })
	return out
}
