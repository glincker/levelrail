package dbintegrations

import (
	"context"
	"log/slog"
)

// CardStatus is one catalog card with its state for one database.
type CardStatus struct {
	Card
	State State `json:"state"`
}

// Service joins the registry with the registered providers.
type Service struct {
	providers map[string]Provider
	logger    *slog.Logger
}

// NewService builds a service from providers keyed by their ID.
func NewService(logger *slog.Logger, providers ...Provider) *Service {
	m := make(map[string]Provider, len(providers))
	for _, p := range providers {
		m[p.ID()] = p
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{providers: m, logger: logger}
}

// List returns every card with its state for db. A provider failure
// degrades that card to unavailable instead of failing the catalog.
func (s *Service) List(ctx context.Context, db Database) ([]CardStatus, error) {
	cards, err := Cards()
	if err != nil {
		return nil, err
	}
	out := make([]CardStatus, 0, len(cards))
	for _, c := range cards {
		out = append(out, CardStatus{Card: c, State: s.state(ctx, c, db)})
	}
	return out, nil
}

func (s *Service) state(ctx context.Context, c Card, db Database) State {
	if !c.SupportsEngine(db.Engine) {
		return State{Code: StateUnavailable, Reason: "not available for this engine"}
	}
	if c.Kind == KindDocs {
		return State{Code: StateDocsOnly}
	}
	p, ok := s.providers[c.ID]
	if !ok {
		return State{Code: StatePlanned, Reason: "connector not built yet"}
	}
	st, err := p.Status(ctx, db)
	if err != nil {
		s.logger.Warn("dbintegrations: status failed", slog.String("integration", c.ID), slog.String("error", err.Error()))
		return State{Code: StateUnavailable, Reason: "status check failed"}
	}
	return st
}
