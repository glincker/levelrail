package maintenance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SilenceCreatorPrefix marks a silence owned by domain maintenance; the rest
// of CreatedBy is the domain. Users cannot forge it: manual silences carry a user name.
const SilenceCreatorPrefix = "maintenance:"

const (
	// openEndedSilence is how long a silence for maintenance with no end lasts before Sync renews it.
	openEndedSilence = 7 * 24 * time.Hour
	renewBefore      = 24 * time.Hour
	silenceListLimit = 1000
)

// silencedKinds are the alert kinds that probe a domain or its readiness, so
// the maintenance 503 would trip them. Crashloops still alert: a real crash is not maintenance.
var silencedKinds = []string{
	string(alerting.KindDomainHealth), string(alerting.KindDomainNotResolving),
	string(alerting.KindCertExpiry), string(alerting.KindCertExpiring), string(alerting.KindCertRenewalStalled),
	string(alerting.KindThreshold),
}

// SilenceStore is the part of *alerting.DB the coupling needs.
type SilenceStore interface {
	CreateSilence(ctx context.Context, s alerting.Silence) error
	ListSilences(ctx context.Context, now time.Time, includeExpired bool, limit int) ([]alerting.Silence, error)
	ExpireSilence(ctx context.Context, id string, now time.Time) (*alerting.Silence, error)
}

// Silencer keeps one alert silence per domain in maintenance.
type Silencer struct {
	Store SilenceStore
}

// SilenceEnd is when the silence for m stops: its end time, its auto-off, or a renewable default.
func SilenceEnd(m store.DomainMaintenance, now time.Time) time.Time {
	switch {
	case !m.EndsAt.IsZero():
		return m.EndsAt
	case !m.AutoOffAt.IsZero():
		return m.AutoOffAt
	}
	return now.Add(openEndedSilence)
}

// Open creates the silence for m on app and returns its id. It never touches
// older silences: callers Settle once the maintenance row is saved.
func (s *Silencer) Open(ctx context.Context, m store.DomainMaintenance, app string, now time.Time) (string, error) {
	id, err := alerting.NewSilenceID()
	if err != nil {
		return "", fmt.Errorf("maintenance: %w", err)
	}
	sil := alerting.Silence{
		ID: id, Matchers: alerting.SilenceMatcher{Apps: []string{app}, Kinds: silencedKinds},
		StartsAt: now, EndsAt: SilenceEnd(m, now), CreatedBy: SilenceCreatorPrefix + m.Domain,
		Reason: "Maintenance on " + m.Domain, CreatedAt: now,
	}
	if err := s.Store.CreateSilence(ctx, sil); err != nil {
		return "", fmt.Errorf("maintenance: silence alerts for %s: %w", m.Domain, err)
	}
	return id, nil
}

// Settle expires every active maintenance silence of domain except keep.
// An empty keep expires them all.
func (s *Silencer) Settle(ctx context.Context, domain, keep string, now time.Time) error {
	owned, err := s.owned(ctx, now)
	if err != nil {
		return err
	}
	var firstErr error
	for _, sil := range owned[domain] {
		if sil.ID == keep {
			continue
		}
		if _, err := s.Store.ExpireSilence(ctx, sil.ID, now); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("maintenance: lift silence %s: %w", sil.ID, err)
		}
	}
	return firstErr
}

// Active returns the active silence for domain, if any.
func (s *Silencer) Active(ctx context.Context, domain string, now time.Time) (*alerting.Silence, error) {
	owned, err := s.owned(ctx, now)
	if err != nil {
		return nil, err
	}
	if list := owned[domain]; len(list) > 0 {
		return &list[0], nil
	}
	return nil, nil
}

// Sync makes silences match active: one fresh silence per row, none for a
// domain that is no longer in maintenance. It heals a crash between the two
// writes and an expiry that happened while the control plane was down.
// apps maps domain to its owning app; a row with no owner is left to the caller.
func (s *Silencer) Sync(ctx context.Context, active []store.DomainMaintenance, apps map[string]string, now time.Time) error {
	owned, err := s.owned(ctx, now)
	if err != nil {
		return err
	}
	live := make(map[string]bool, len(active))
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, m := range active {
		live[m.Domain] = true
		app, ok := apps[m.Domain]
		if !ok || silenceCurrent(owned[m.Domain], m, now) {
			continue
		}
		id, err := s.Open(ctx, m, app, now)
		if err != nil {
			note(err)
			continue
		}
		note(s.Settle(ctx, m.Domain, id, now))
	}
	for domain, list := range owned {
		if live[domain] {
			continue
		}
		for _, sil := range list {
			if _, err := s.Store.ExpireSilence(ctx, sil.ID, now); err != nil {
				note(fmt.Errorf("maintenance: lift silence %s: %w", sil.ID, err))
			}
		}
	}
	return firstErr
}

func silenceCurrent(list []alerting.Silence, m store.DomainMaintenance, now time.Time) bool {
	if len(list) != 1 {
		return false
	}
	end := list[0].EndsAt
	if fixed := !m.EndsAt.IsZero() || !m.AutoOffAt.IsZero(); fixed {
		return !end.Before(SilenceEnd(m, now).Add(-time.Second))
	}
	return end.Sub(now) > renewBefore
}

func (s *Silencer) owned(ctx context.Context, now time.Time) (map[string][]alerting.Silence, error) {
	all, err := s.Store.ListSilences(ctx, now, false, silenceListLimit)
	if err != nil {
		return nil, fmt.Errorf("maintenance: list silences: %w", err)
	}
	out := map[string][]alerting.Silence{}
	for _, sil := range all {
		if domain, ok := strings.CutPrefix(sil.CreatedBy, SilenceCreatorPrefix); ok && sil.Status(now) != alerting.SilenceExpired {
			out[domain] = append(out[domain], sil)
		}
	}
	return out, nil
}

// SilenceDomain returns the domain a maintenance silence belongs to.
func SilenceDomain(createdBy string) (string, bool) {
	return strings.CutPrefix(createdBy, SilenceCreatorPrefix)
}

// Discard expires one silence by id, for undoing an Open that was not committed.
func (s *Silencer) Discard(ctx context.Context, id string, now time.Time) error {
	if _, err := s.Store.ExpireSilence(ctx, id, now); err != nil {
		return fmt.Errorf("maintenance: lift silence %s: %w", id, err)
	}
	return nil
}
