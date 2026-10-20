package models

import "sync"

// swapGroupLocks serializes residency decisions within one swap_group,
// so two models sharing a GPU can never both decide to evict each other
// at once. Process-local: the control plane is a single process
// (CLAUDE.md section 4.1), never sharded across nodes, so this is
// sufficient without a distributed lock.
var (
	swapGroupLocksMu sync.Mutex
	swapGroupLocks   = map[string]*sync.Mutex{}
)

// lockSwapGroup blocks until group's lock is free, then returns the
// function that releases it.
func lockSwapGroup(group string) func() {
	swapGroupLocksMu.Lock()
	l, ok := swapGroupLocks[group]
	if !ok {
		l = &sync.Mutex{}
		swapGroupLocks[group] = l
	}
	swapGroupLocksMu.Unlock()

	l.Lock()
	return l.Unlock
}
