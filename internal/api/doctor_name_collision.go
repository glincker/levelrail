package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

type nameCollisionAuditor interface {
	NameCollisions(ctx context.Context) ([]store.NameCollision, error)
}

// doctorCheckNameNamespaceCollision lists names held by more than one kind of resource (read only).
func (rt *Router) doctorCheckNameNamespaceCollision(ctx context.Context) doctorCheckResource {
	const code, name = "name_namespace_collision", "Shared name namespace"
	a, ok := rt.apps.(nameCollisionAuditor)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Names could not be audited on this store."}
	}
	found, err := a.NameCollisions(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("Names could not be read: %s", err)}
	}
	if len(found) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No app, database or stack shares a name."}
	}
	parts := make([]string, len(found))
	for i, c := range found {
		parts[i] = c.String()
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d name(s) are used by more than one kind of resource and share one secret namespace: %s", len(found), strings.Join(parts, ", ")),
		Fix:     "Copy what you need into a uniquely named resource and delete the duplicate.",
	}
}
