package api

import (
	"context"
	"errors"
	"slices"

	"github.com/GLINCKER/levelrail/internal/store"
)

// recordDomainMoveRun logs an app-to-app move in the default sets so the
// rollback window can hand the domain back. Other shapes are not undoable.
func (rt *Router) recordDomainMoveRun(ctx context.Context, domain string, from, to lifecycleSide) {
	if from.target.Kind != store.DomainKindApp || to.target.Kind != store.DomainKindApp || from.envName != "" || to.envName != "" {
		return
	}
	res := &goLiveResult{
		App: to.target.Name, Domain: domain, State: "moved",
		undo: []undoAction{{Kind: undoKindMoveDomain, App: from.target.Name, To: to.target.Name, Domain: domain}},
	}
	rt.recordGoLiveRun(ctx, res)
}

// undoMoveDomain moves the host back, but only while the target app still
// serves it and the origin has not claimed it again, so a later edit is kept.
func (rt *Router) undoMoveDomain(ctx context.Context, u undoAction) (string, error) {
	label := "move " + u.Domain + " back to " + u.App
	st := rt.parkedStore()
	if st == nil {
		return label, errors.New("not supported")
	}
	if !slices.Contains(rt.appDomainList(ctx, u.To), u.Domain) {
		return label, errors.New("the target app no longer serves this domain")
	}
	err := st.MoveDomainBetweenTargets(ctx, u.Domain,
		store.DomainTarget{Kind: store.DomainKindApp, Name: u.To},
		store.DomainTarget{Kind: store.DomainKindApp, Name: u.App})
	if err != nil {
		return label, err
	}
	return "moved " + u.Domain + " back to " + u.App, nil
}
