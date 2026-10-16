package maintenance

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

var sT0 = time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)

type memSilences struct {
	list []alerting.Silence
}

func (m *memSilences) CreateSilence(_ context.Context, s alerting.Silence) error {
	m.list = append(m.list, s)
	return nil
}

func (m *memSilences) ListSilences(_ context.Context, now time.Time, _ bool, _ int) ([]alerting.Silence, error) {
	var out []alerting.Silence
	for _, s := range m.list {
		if s.Status(now) != alerting.SilenceExpired {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memSilences) ExpireSilence(_ context.Context, id string, now time.Time) (*alerting.Silence, error) {
	for i := range m.list {
		if m.list[i].ID == id {
			t := now
			m.list[i].ExpiredAt = &t
			return &m.list[i], nil
		}
	}
	return nil, alerting.ErrSilenceNotFound
}

func (m *memSilences) active(now time.Time) int {
	n := 0
	for _, s := range m.list {
		if s.Status(now) == alerting.SilenceActive {
			n++
		}
	}
	return n
}

func TestSilencer_OpenSettleSync(t *testing.T) {
	ctx := context.Background()
	row := store.DomainMaintenance{Domain: "a.example.com", StartedAt: sT0}
	apps := map[string]string{"a.example.com": "web"}

	tests := []struct {
		name string
		run  func(t *testing.T, s *Silencer, m *memSilences)
	}{
		{"enable twice keeps one", func(t *testing.T, s *Silencer, m *memSilences) {
			first, _ := s.Open(ctx, row, "web", sT0)
			second, _ := s.Open(ctx, row, "web", sT0)
			if err := s.Settle(ctx, row.Domain, second, sT0); err != nil {
				t.Fatal(err)
			}
			if m.active(sT0) != 1 || first == second {
				t.Fatalf("active = %d", m.active(sT0))
			}
		}},
		{"disable never enabled", func(t *testing.T, s *Silencer, m *memSilences) {
			if err := s.Settle(ctx, "never.example.com", "", sT0); err != nil || m.active(sT0) != 0 {
				t.Fatalf("err=%v active=%d", err, m.active(sT0))
			}
		}},
		{"restart heals a missing silence", func(t *testing.T, _ *Silencer, m *memSilences) {
			if err := (&Silencer{Store: m}).Sync(ctx, []store.DomainMaintenance{row}, apps, sT0); err != nil {
				t.Fatal(err)
			}
			if m.active(sT0) != 1 {
				t.Fatalf("active = %d, want 1", m.active(sT0))
			}
		}},
		{"sync is idempotent", func(t *testing.T, s *Silencer, m *memSilences) {
			for i := 0; i < 3; i++ {
				_ = s.Sync(ctx, []store.DomainMaintenance{row}, apps, sT0.Add(time.Duration(i)*time.Minute))
			}
			if m.active(sT0.Add(3*time.Minute)) != 1 || len(m.list) != 1 {
				t.Fatalf("silences = %d", len(m.list))
			}
		}},
		{"expiry while down lifts the silence", func(t *testing.T, s *Silencer, m *memSilences) {
			_, _ = s.Open(ctx, row, "web", sT0)
			if err := s.Sync(ctx, nil, apps, sT0.Add(time.Hour)); err != nil || m.active(sT0.Add(time.Hour)) != 0 {
				t.Fatalf("err=%v active=%d", err, m.active(sT0.Add(time.Hour)))
			}
		}},
		{"open ended silence is renewed", func(t *testing.T, s *Silencer, m *memSilences) {
			_, _ = s.Open(ctx, row, "web", sT0)
			later := sT0.Add(openEndedSilence - time.Hour)
			_ = s.Sync(ctx, []store.DomainMaintenance{row}, apps, later)
			if m.active(later) != 1 || len(m.list) != 2 {
				t.Fatalf("active=%d total=%d", m.active(later), len(m.list))
			}
		}},
		{"manual silences are never touched", func(t *testing.T, s *Silencer, m *memSilences) {
			m.list = append(m.list, alerting.Silence{ID: "sil_x", CreatedBy: "ada", StartsAt: sT0, EndsAt: sT0.Add(time.Hour)})
			_ = s.Sync(ctx, nil, apps, sT0)
			if m.active(sT0) != 1 {
				t.Fatal("manual silence was lifted")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &memSilences{}
			tt.run(t, &Silencer{Store: m}, m)
		})
	}
}

type fakeOwnerStore struct {
	fakeExpiryStore
	active  []store.DomainMaintenance
	owners  []store.ServiceDomain
	resolve []string
}

func (f *fakeOwnerStore) ListActiveDomainMaintenance(context.Context, time.Time) ([]store.DomainMaintenance, error) {
	return f.active, nil
}

func (f *fakeOwnerStore) ListServiceDomains(context.Context) ([]store.ServiceDomain, error) {
	return f.owners, nil
}

func (f *fakeOwnerStore) EndOngoingMaintenanceWindow(_ context.Context, id string, _ time.Time) error {
	f.resolve = append(f.resolve, id)
	return nil
}

func TestDomainExpirer_EndPathsLiftSilenceAndAudit(t *testing.T) {
	owned := []store.ServiceDomain{{Domain: "a.example.com", ServiceName: "web"}}
	tests := []struct {
		name       string
		expired    []store.DomainMaintenance
		active     []store.DomainMaintenance
		owners     []store.ServiceDomain
		wantReason string
		wantWindow bool
	}{
		{"until", []store.DomainMaintenance{{Domain: "a.example.com", StartedAt: sT0.Add(-time.Hour), WindowID: "mw_1", EndsAt: sT0.Add(-time.Minute)}}, nil, owned, EndUntil, false},
		{"max duration", []store.DomainMaintenance{{Domain: "a.example.com", StartedAt: sT0.Add(-time.Hour), WindowID: "mw_2", Ongoing: true, AutoOffAt: sT0.Add(-time.Minute)}}, nil, owned, EndMaxDuration, true},
		{"domain removed", nil, []store.DomainMaintenance{{Domain: "a.example.com", StartedAt: sT0.Add(-time.Hour), WindowID: "mw_3", Ongoing: true}}, nil, EndDomainRemoved, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sil := &memSilences{}
			sl := &Silencer{Store: sil}
			_, _ = sl.Open(context.Background(), store.DomainMaintenance{Domain: "a.example.com"}, "web", sT0.Add(-time.Hour))
			st := &fakeOwnerStore{fakeExpiryStore: fakeExpiryStore{expired: tt.expired}, active: tt.active, owners: tt.owners}
			x := &DomainExpirer{Store: st, Silencer: sl, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return sT0 }}
			if err := x.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(st.audits) != 1 || !jsonHas(st.audits[0].Detail, `"end_reason":"`+tt.wantReason+`"`) || !jsonHas(st.audits[0].Detail, `"duration_seconds":3600`) {
				t.Fatalf("audits = %+v", st.audits)
			}
			if sil.active(sT0) != 0 {
				t.Fatal("silence not lifted")
			}
			if tt.wantWindow != (len(st.resolve) == 1) {
				t.Fatalf("resolved windows = %v", st.resolve)
			}
		})
	}
}

func jsonHas(s, part string) bool { return strings.Contains(s, part) }

func (f *fakeOwnerStore) DeleteDomainMaintenance(ctx context.Context, d string) error {
	kept := f.active[:0]
	for _, m := range f.active {
		if m.Domain != d {
			kept = append(kept, m)
		}
	}
	f.active = kept
	return f.fakeExpiryStore.DeleteDomainMaintenance(ctx, d)
}
