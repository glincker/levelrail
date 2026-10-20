package cutover

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RunRows is the control plane database surface DBStore needs; *store.DB
// provides it.
type RunRows interface {
	CreateCutoverRun(ctx context.Context, r store.CutoverRun) error
	SaveCutoverRun(ctx context.Context, r store.CutoverRun) error
	GetCutoverRun(ctx context.Context, id string) (store.CutoverRun, error)
	ListCutoverRuns(ctx context.Context, app string, limit int) ([]store.CutoverRun, error)
	ListCutoverRunsByState(ctx context.Context, states ...string) ([]store.CutoverRun, error)
}

// DBStore persists runs in the control plane database.
type DBStore struct{ DB RunRows }

func toRow(r Run) (store.CutoverRun, error) {
	domains, err := json.Marshal(nonNil(r.Domains))
	if err != nil {
		return store.CutoverRun{}, fmt.Errorf("cutover: encode domains: %w", err)
	}
	steps, err := json.Marshal(nonNil(r.Steps))
	if err != nil {
		return store.CutoverRun{}, fmt.Errorf("cutover: encode steps: %w", err)
	}
	plan := ""
	if r.Plan != nil {
		b, err := json.Marshal(r.Plan)
		if err != nil {
			return store.CutoverRun{}, fmt.Errorf("cutover: encode plan: %w", err)
		}
		plan = string(b)
	}
	return store.CutoverRun{ID: r.ID, SessionID: r.SessionID, SourceID: r.SourceID, AppName: r.App, Mode: r.Mode, State: r.State,
		Method: r.Method, DNSWrite: r.DNSWrite, WasRouted: r.WasRouted, AcceptWarn: r.AcceptWarn, Awaiting: r.Awaiting,
		DomainsJSON: string(domains), StepsJSON: string(steps), PlanJSON: plan, Error: r.Error,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, FinishedAt: r.FinishedAt}, nil
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func fromRow(row store.CutoverRun) (Run, error) {
	r := Run{ID: row.ID, SessionID: row.SessionID, SourceID: row.SourceID, App: row.AppName, Mode: row.Mode, State: row.State,
		Method: row.Method, DNSWrite: row.DNSWrite, WasRouted: row.WasRouted, AcceptWarn: row.AcceptWarn, Awaiting: row.Awaiting,
		Error: row.Error, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, FinishedAt: row.FinishedAt}
	if err := json.Unmarshal([]byte(row.DomainsJSON), &r.Domains); err != nil {
		return Run{}, fmt.Errorf("cutover: decode domains of %q: %w", row.ID, err)
	}
	if err := json.Unmarshal([]byte(row.StepsJSON), &r.Steps); err != nil {
		return Run{}, fmt.Errorf("cutover: decode steps of %q: %w", row.ID, err)
	}
	if row.PlanJSON != "" {
		var p Plan
		if err := json.Unmarshal([]byte(row.PlanJSON), &p); err != nil {
			return Run{}, fmt.Errorf("cutover: decode plan of %q: %w", row.ID, err)
		}
		r.Plan = &p
	}
	return r, nil
}

// Create implements Store.
func (s DBStore) Create(ctx context.Context, r Run) error {
	row, err := toRow(r)
	if err != nil {
		return err
	}
	return s.DB.CreateCutoverRun(ctx, row)
}

// Get implements Store.
func (s DBStore) Get(ctx context.Context, id string) (Run, error) {
	row, err := s.DB.GetCutoverRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	return fromRow(row)
}

// Save implements Store.
func (s DBStore) Save(ctx context.Context, r Run) error {
	row, err := toRow(r)
	if err != nil {
		return err
	}
	return s.DB.SaveCutoverRun(ctx, row)
}

func fromRows(rows []store.CutoverRun) ([]Run, error) {
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

// List implements Store.
func (s DBStore) List(ctx context.Context, app string, limit int) ([]Run, error) {
	rows, err := s.DB.ListCutoverRuns(ctx, app, limit)
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}

// ListByState implements Store.
func (s DBStore) ListByState(ctx context.Context, states ...string) ([]Run, error) {
	rows, err := s.DB.ListCutoverRunsByState(ctx, states...)
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}
