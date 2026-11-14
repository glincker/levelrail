package stacksync

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Trigger outcomes: queued started a run, coalesced folded into the pending one.
const (
	TriggerQueued    = "queued"
	TriggerCoalesced = "coalesced"
)

// triggerSlot is one stack's background run and at most one pending follow up.
type triggerSlot struct {
	running   bool
	pending   *store.StackSyncRecord
	wantApply bool
}

// Trigger syncs rec in the background and returns at once. apply false plans
// only; true applies when the stack itself sets apply. Triggers that arrive
// while a run is going collapse into one pending run, so a burst costs two.
func (s *Syncer) Trigger(rec store.StackSyncRecord, apply bool) string {
	s.mu.Lock()
	if s.slots == nil {
		s.slots = map[string]*triggerSlot{}
	}
	slot := s.slots[rec.StackID]
	if slot == nil {
		slot = &triggerSlot{}
		s.slots[rec.StackID] = slot
	}
	if slot.running {
		latest := rec
		slot.pending = &latest
		slot.wantApply = slot.wantApply || apply
		s.mu.Unlock()
		return TriggerCoalesced
	}
	slot.running = true
	ctx := s.base
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	go s.drain(ctx, rec, apply)
	return TriggerQueued
}

func (s *Syncer) drain(ctx context.Context, rec store.StackSyncRecord, apply bool) {
	for {
		run := rec
		run.Config.Apply = run.Config.Apply && apply
		s.SyncOne(ctx, run)
		s.mu.Lock()
		slot := s.slots[rec.StackID]
		if slot == nil || slot.pending == nil {
			delete(s.slots, rec.StackID)
			s.mu.Unlock()
			return
		}
		rec, apply = *slot.pending, slot.wantApply
		slot.pending, slot.wantApply = nil, false
		s.mu.Unlock()
	}
}

// Busy reports whether a triggered run of the stack is going or pending.
func (s *Syncer) Busy(stackID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.slots[stackID]
	return ok
}
