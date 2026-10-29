package statuspage

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// WindowSource lists one-off maintenance windows. *store.DB satisfies it.
type WindowSource interface {
	ListMaintenanceWindows(ctx context.Context, since time.Time) ([]store.MaintenanceWindow, error)
}

// SetWindows adds scheduled maintenance windows to the public view.
func (s *Service) SetWindows(w WindowSource) { s.windows = w }

// withWindows lists each window as scheduled maintenance. Only the title and
// message are public: the scope target (an app or project name) is not.
func (s *Service) withWindows(ctx context.Context, v View, now time.Time, window time.Duration) View {
	if s.windows == nil {
		return v
	}
	ws, err := s.windows.ListMaintenanceWindows(ctx, now.Add(-window))
	if err != nil {
		s.logger.Warn("statuspage: list maintenance windows failed", "error", err.Error())
		return v
	}
	allActive := false
	for _, w := range ws {
		end := w.EndsAt
		status := StatusScheduled
		switch {
		case w.Ongoing:
			status = StatusInProgress
			allActive = allActive || w.Scope == store.MaintenanceScopeAll
		case !now.Before(w.EndsAt):
			status = StatusCompleted
		case !now.Before(w.StartsAt):
			status = StatusInProgress
			allActive = allActive || w.Scope == store.MaintenanceScopeAll
		}
		updates := []UpdateView{}
		if w.Message != "" {
			updates = append(updates, UpdateView{Status: status, Body: w.Message, At: w.CreatedAt.UTC()})
		}
		view := IncidentView{ID: w.ID, Title: w.Title, Status: status,
			Impact: Maintenance, StartsAt: w.StartsAt.UTC(), EndsAt: &end, Updates: updates}
		if w.Ongoing {
			view.EndsAt, view.Ongoing = nil, true
		}
		v.Maintenance = append(v.Maintenance, view)
	}
	if allActive && len(v.Components) > 0 {
		statuses := make([]string, len(v.Components))
		for i := range v.Components {
			v.Components[i].Status = Maintenance
			statuses[i] = Maintenance
		}
		v.Status = OverallStatus(statuses)
		v.StatusText = StatusText(v.Status)
	}
	return v
}
