package takeover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Rows is the control plane database surface DBStore needs; *store.DB provides it.
type Rows interface {
	CreatePortTakeover(ctx context.Context, r store.PortTakeover) error
	SavePortTakeover(ctx context.Context, r store.PortTakeover) error
	GetPortTakeover(ctx context.Context, id string) (store.PortTakeover, error)
	ListPortTakeovers(ctx context.Context, limit int) ([]store.PortTakeover, error)
	ListPortTakeoversByState(ctx context.Context, states ...string) ([]store.PortTakeover, error)
}

// DBStore persists runs in the control plane database.
type DBStore struct{ DB Rows }

func toRow(r Run) (store.PortTakeover, error) {
	prev, err := json.Marshal(r.Previous)
	if err != nil {
		return store.PortTakeover{}, fmt.Errorf("takeover: encode previous: %w", err)
	}
	steps, err := json.Marshal(nonNil(r.Steps))
	if err != nil {
		return store.PortTakeover{}, fmt.Errorf("takeover: encode steps: %w", err)
	}
	verify, err := json.Marshal(nonNil(r.Verify))
	if err != nil {
		return store.PortTakeover{}, fmt.Errorf("takeover: encode verify: %w", err)
	}
	return store.PortTakeover{ID: r.ID, State: r.State, Actor: r.Actor, PreviousJSON: string(prev), TargetHTTP: r.TargetHTTP, TargetHTTPS: r.TargetHTTPS,
		StepsJSON: string(steps), VerifyJSON: string(verify), Error: r.Error, Boots: r.Boots, WindowSeconds: r.WindowSeconds, DeadlineAt: r.DeadlineAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, FinishedAt: r.FinishedAt}, nil
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func fromRow(row store.PortTakeover) (Run, error) {
	r := Run{ID: row.ID, State: row.State, Actor: row.Actor, TargetHTTP: row.TargetHTTP, TargetHTTPS: row.TargetHTTPS, Error: row.Error, Boots: row.Boots,
		WindowSeconds: row.WindowSeconds, DeadlineAt: row.DeadlineAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
	if err := json.Unmarshal([]byte(row.PreviousJSON), &r.Previous); err != nil {
		return Run{}, fmt.Errorf("takeover: decode previous of %q: %w", row.ID, err)
	}
	if err := json.Unmarshal([]byte(row.StepsJSON), &r.Steps); err != nil {
		return Run{}, fmt.Errorf("takeover: decode steps of %q: %w", row.ID, err)
	}
	if err := json.Unmarshal([]byte(row.VerifyJSON), &r.Verify); err != nil {
		return Run{}, fmt.Errorf("takeover: decode verify of %q: %w", row.ID, err)
	}
	return r, nil
}

func fromRows(rows []store.PortTakeover, err error) ([]Run, error) {
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		r, err := fromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Create implements Store.
func (s DBStore) Create(ctx context.Context, r Run) error {
	row, err := toRow(r)
	if err != nil {
		return err
	}
	return s.DB.CreatePortTakeover(ctx, row)
}

// Get implements Store.
func (s DBStore) Get(ctx context.Context, id string) (Run, error) {
	row, err := s.DB.GetPortTakeover(ctx, id)
	if err != nil {
		return Run{}, mapNotFound(err)
	}
	return fromRow(row)
}

// Save implements Store.
func (s DBStore) Save(ctx context.Context, r Run) error {
	row, err := toRow(r)
	if err != nil {
		return err
	}
	return mapNotFound(s.DB.SavePortTakeover(ctx, row))
}

// List implements Store.
func (s DBStore) List(ctx context.Context, limit int) ([]Run, error) {
	return fromRows(s.DB.ListPortTakeovers(ctx, limit))
}

// ListByState implements Store.
func (s DBStore) ListByState(ctx context.Context, states ...string) ([]Run, error) {
	return fromRows(s.DB.ListPortTakeoversByState(ctx, states...))
}

func mapNotFound(err error) error {
	if errors.Is(err, store.ErrPortTakeoverNotFound) {
		return ErrNotFound
	}
	return err
}
