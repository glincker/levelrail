// Package maintenance holds one-off scheduled maintenance windows: their
// validation, state, and the lead-time announcement through alert channels.
package maintenance

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Window states relative to one instant.
const (
	StateScheduled = "scheduled"
	StateActive    = "active"
	StateEnded     = "ended"
)

// Bounds on user input.
const (
	MaxDuration     = 30 * 24 * time.Hour
	MaxLeadMinutes  = 7 * 24 * 60
	maxTitleLength  = 120
	maxMessageBytes = 2000
)

// Input is a request to create a window.
type Input struct {
	Title            string
	Message          string
	Scope            string
	Target           string
	StartsAt         time.Time
	EndsAt           time.Time
	Freeze           bool
	AnnounceLeadMins int
}

// Validate checks the input against now and returns the window to store.
func (in Input) Validate(now time.Time) (store.MaintenanceWindow, error) {
	title := strings.TrimSpace(in.Title)
	switch {
	case title == "":
		return store.MaintenanceWindow{}, errors.New("title is required")
	case len(title) > maxTitleLength:
		return store.MaintenanceWindow{}, fmt.Errorf("title must be at most %d characters", maxTitleLength)
	case len(in.Message) > maxMessageBytes:
		return store.MaintenanceWindow{}, fmt.Errorf("message must be at most %d bytes", maxMessageBytes)
	case in.StartsAt.IsZero() || in.EndsAt.IsZero():
		return store.MaintenanceWindow{}, errors.New("starts_at and ends_at are required")
	case !in.EndsAt.After(in.StartsAt):
		return store.MaintenanceWindow{}, errors.New("ends_at must be after starts_at")
	case in.EndsAt.Sub(in.StartsAt) > MaxDuration:
		return store.MaintenanceWindow{}, fmt.Errorf("a window may last at most %s", MaxDuration)
	case !in.EndsAt.After(now):
		return store.MaintenanceWindow{}, errors.New("ends_at is already in the past")
	case in.AnnounceLeadMins < 0 || in.AnnounceLeadMins > MaxLeadMinutes:
		return store.MaintenanceWindow{}, fmt.Errorf("announce_lead_minutes must be between 0 and %d", MaxLeadMinutes)
	}
	target := strings.TrimSpace(in.Target)
	switch in.Scope {
	case store.MaintenanceScopeAll:
		target = ""
	case store.MaintenanceScopeProject, store.MaintenanceScopeApp:
		if target == "" {
			return store.MaintenanceWindow{}, fmt.Errorf("target is required for scope %q", in.Scope)
		}
	default:
		return store.MaintenanceWindow{}, fmt.Errorf("scope must be %q, %q or %q",
			store.MaintenanceScopeAll, store.MaintenanceScopeProject, store.MaintenanceScopeApp)
	}
	return store.MaintenanceWindow{
		Title: title, Message: strings.TrimSpace(in.Message), Scope: in.Scope, Target: target,
		StartsAt: in.StartsAt.UTC(), EndsAt: in.EndsAt.UTC(), Freeze: in.Freeze,
		AnnounceLeadMins: in.AnnounceLeadMins,
	}, nil
}

// StateAt reports where w is relative to now; the end instant is exclusive.
func StateAt(w store.MaintenanceWindow, now time.Time) string {
	switch {
	case now.Before(w.StartsAt):
		return StateScheduled
	case now.Before(w.EndsAt):
		return StateActive
	default:
		return StateEnded
	}
}

// AnnounceDue reports whether the lead-time notice should go out now.
func AnnounceDue(w store.MaintenanceWindow, now time.Time) bool {
	if w.AnnounceLeadMins <= 0 || w.AnnouncedAt != nil || !now.Before(w.EndsAt) {
		return false
	}
	return !now.Before(w.StartsAt.Add(-time.Duration(w.AnnounceLeadMins) * time.Minute))
}
