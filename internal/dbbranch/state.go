// Package dbbranch holds the lifecycle rules of database branches: states, engine capabilities, quotas and expiry selection.
package dbbranch

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Lifecycle states of a branch.
const (
	StateRequested   = "requested"
	StateCloning     = "cloning"
	StateMasking     = "masking"
	StateCredentials = "credentials"
	StateVerifying   = "verifying"
	StateReady       = "ready"
	StateResetting   = "resetting"
	StatePromoting   = "promoting"
	StatePromoted    = "promoted"
	StateExpiring    = "expiring"
	StateDeleting    = "deleting"
	StateDeleted     = "deleted"
	StateFailed      = "failed"
)

// ErrBadTransition is returned for a state move the lifecycle does not allow.
var ErrBadTransition = errors.New("dbbranch: transition not allowed")

var transitions = map[string][]string{
	StateRequested:   {StateCloning},
	StateCloning:     {StateMasking, StateCredentials},
	StateMasking:     {StateCredentials},
	StateCredentials: {StateVerifying},
	StateVerifying:   {StateReady},
	StateReady:       {StateResetting, StatePromoting, StateExpiring, StateDeleting},
	StateResetting:   {StateReady},
	StatePromoting:   {StatePromoted},
	StateExpiring:    {StateDeleting},
	StateDeleting:    {StateDeleted},
	StateFailed:      {StateResetting, StateExpiring, StateDeleting},
	// promoted has no moves: it is a standalone database deleted through the normal delete path, which removes the row.
}

// CanTransition reports whether from may move to to. Any step before ready and any in flight step may fail.
func CanTransition(from, to string) bool {
	if to == StateFailed {
		switch from {
		case StateRequested, StateCloning, StateMasking, StateCredentials, StateVerifying, StateResetting, StatePromoting, StateExpiring, StateDeleting:
			return true
		}
		return false
	}
	for _, n := range transitions[from] {
		if n == to {
			return true
		}
	}
	return false
}

// Reachable reports whether the branch may serve clients: only ready or promoted.
func Reachable(state string) bool { return state == StateReady || state == StatePromoted }

// Store is the part of the branch store the lifecycle needs.
type Store interface {
	UpdateDatabaseBranchState(ctx context.Context, id, from, to, reason, now string) error
}

// Move validates the transition and applies it as a compare and set on (id, state).
func Move(ctx context.Context, s Store, id, from, to, reason, now string) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s to %s", ErrBadTransition, from, to)
	}
	return s.UpdateDatabaseBranchState(ctx, id, from, to, reason, now)
}

var _ Store = (*store.DB)(nil)

var _ CloneStore = (*store.DB)(nil)
