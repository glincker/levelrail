package maintenance

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDomainInput_Validate(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	soon := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	far := now.Add(MaxDuration + time.Hour)
	cases := []struct {
		name    string
		in      DomainInput
		wantErr string
	}{
		{"defaults", DomainInput{}, ""},
		{"full", DomainInput{Title: " Upgrade ", Message: "Back soon", RetryAfterSeconds: 60, Until: &soon, Bypass: []string{"10.0.0.1"}}, ""},
		{"long title", DomainInput{Title: strings.Repeat("a", MaxDomainTitle+1)}, "title"},
		{"long message", DomainInput{Message: strings.Repeat("a", MaxDomainMessage+1)}, "message"},
		{"negative retry", DomainInput{RetryAfterSeconds: -1}, "retry_after"},
		{"retry too long", DomainInput{RetryAfterSeconds: MaxRetryAfterSecond + 1}, "retry_after"},
		{"until in the past", DomainInput{Until: &past}, "past"},
		{"until too far", DomainInput{Until: &far}, "at most"},
		{"bad bypass", DomainInput{Bypass: []string{"not-an-ip"}}, "not an IP"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := tc.in.Validate("a.example.com", now)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || m.Domain != "a.example.com" || m.Title != strings.TrimSpace(tc.in.Title) {
				t.Fatalf("Validate() = %+v, %v", m, err)
			}
		})
	}
}

func TestNormalizeBypass(t *testing.T) {
	got, err := NormalizeBypass([]string{"10.1.2.3", " 10.1.2.3/32 ", "192.168.1.77/24", "2001:db8::1", "", "::ffff:198.51.100.4"})
	want := []string{"10.1.2.3/32", "192.168.1.0/24", "2001:db8::1/128", "198.51.100.4/32"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeBypass() = %v, %v; want %v", got, err, want)
	}
	for _, bad := range [][]string{{"0.0.0.0/0"}, {"::/0"}, {"10.0.0.0/33"}, {"example.com"}} {
		if _, err := NormalizeBypass(bad); err == nil {
			t.Errorf("NormalizeBypass(%v) accepted", bad)
		}
	}
	many := make([]string, MaxBypassEntries+1)
	for i := range many {
		many[i] = "10.0.0.1"
	}
	if _, err := NormalizeBypass(many); err == nil {
		t.Error("more than the cap accepted")
	}
}

type fakeExpiryStore struct {
	expired []store.DomainMaintenance
	deleted []string
	audits  []store.AuditEntry
}

func (f *fakeExpiryStore) ListExpiredDomainMaintenance(context.Context, time.Time) ([]store.DomainMaintenance, error) {
	return f.expired, nil
}

func (f *fakeExpiryStore) DeleteDomainMaintenance(_ context.Context, d string) error {
	f.deleted = append(f.deleted, d)
	return nil
}

func (f *fakeExpiryStore) SaveAuditEntry(_ context.Context, e store.AuditEntry) error {
	f.audits = append(f.audits, e)
	return nil
}

func TestDomainExpirer_EndsAuditsAndNudges(t *testing.T) {
	st := &fakeExpiryStore{expired: []store.DomainMaintenance{{Domain: "a.example.com", WindowID: "mw_1"}}}
	nudged := 0
	x := &DomainExpirer{Store: st, Nudge: func() { nudged++ }, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := x.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st.deleted, []string{"a.example.com"}) || nudged != 1 {
		t.Errorf("deleted = %v nudged = %d", st.deleted, nudged)
	}
	if len(st.audits) != 1 || st.audits[0].Action != store.AuditActionDomainMaintenanceEnded {
		t.Errorf("audits = %+v", st.audits)
	}
	st.expired, st.audits = nil, nil
	if err := x.Tick(context.Background()); err != nil || nudged != 1 || len(st.audits) != 0 {
		t.Errorf("idle tick nudged=%d audits=%v err=%v", nudged, st.audits, err)
	}
}

func TestDomainWindow(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	w := DomainWindow(store.DomainMaintenance{Domain: "a.example.com"}, "web", now, now.Add(time.Hour))
	if w.Title != "Maintenance on a.example.com" || w.Scope != store.MaintenanceScopeApp || w.Target != "web" || w.Freeze {
		t.Errorf("window = %+v", w)
	}
}
