package secretrotation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestScheduleStatus(t *testing.T) {
	soon := 7 * 24 * time.Hour
	tests := []struct {
		name     string
		last     time.Time
		updated  time.Time
		interval int
		now      time.Time
		want     string
	}{
		{"unknown basis", time.Time{}, time.Time{}, 30, t0, StatusUnknown},
		{"no interval", t0, t0, 0, t0, StatusUnknown},
		{"fresh", t0, t0, 30, t0.AddDate(0, 0, 1), StatusOK},
		{"inside soon window", t0, t0, 30, t0.AddDate(0, 0, 25), StatusDueSoon},
		{"exactly due", t0, t0, 30, t0.AddDate(0, 0, 30), StatusOverdue},
		{"overdue", t0, t0, 30, t0.AddDate(0, 0, 90), StatusOverdue},
		{"manual edit resets clock", t0.AddDate(0, 0, -100), t0.AddDate(0, 0, -1), 30, t0, StatusOK},
		{"policy rotation newer than value", t0, t0.AddDate(0, 0, -50), 30, t0.AddDate(0, 0, 10), StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			due := NextDue(Basis(tt.last, tt.updated), tt.interval)
			if got := StatusAt(tt.now, due, soon); got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

type fakeStore struct {
	policies map[string]store.SecretRotationPolicy
	saveErr  error
}

func (f *fakeStore) k(s, k string) string { return s + "/" + k }
func (f *fakeStore) GetSecretRotationPolicy(_ context.Context, s, k string) (store.SecretRotationPolicy, error) {
	p, ok := f.policies[f.k(s, k)]
	if !ok {
		return p, store.ErrSecretRotationPolicyNotFound
	}
	return p, nil
}
func (f *fakeStore) ListSecretRotationPolicies(_ context.Context, _ string) ([]store.SecretRotationPolicy, error) {
	var out []store.SecretRotationPolicy
	for _, p := range f.policies {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeStore) SaveSecretRotationProgress(_ context.Context, p store.SecretRotationPolicy, _ time.Time) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.policies[f.k(p.ServiceName, p.EnvKey)] = p
	return nil
}
func (f *fakeStore) DeleteSecretRotationPolicy(_ context.Context, s, k string) (bool, error) {
	delete(f.policies, f.k(s, k))
	return true, nil
}

type fakeBackend struct {
	apps       map[string]bool // app -> suspended
	values     map[string]string
	locked     map[string]bool
	updated    map[string]time.Time
	restartErr error
	setErr     map[string]error
	restarts   int
}

func (b *fakeBackend) AppState(_ context.Context, app string) (bool, bool, error) {
	s, ok := b.apps[app]
	return ok, s, nil
}
func (b *fakeBackend) Value(_ context.Context, app, key string) (string, error) {
	v, ok := b.values[app+"/"+key]
	if !ok {
		return "", errors.New("missing")
	}
	return v, nil
}
func (b *fakeBackend) SetValue(_ context.Context, app, key, v string) error {
	if err := b.setErr[key]; err != nil {
		return err
	}
	b.values[app+"/"+key] = v
	return nil
}
func (b *fakeBackend) DeleteValue(_ context.Context, app, key string) error {
	delete(b.values, app+"/"+key)
	return nil
}
func (b *fakeBackend) KeyState(_ context.Context, app, key string) (bool, bool, time.Time, error) {
	_, ok := b.values[app+"/"+key]
	return ok, b.locked[app+"/"+key], b.updated[app+"/"+key], nil
}
func (b *fakeBackend) Restart(_ context.Context, _ string) error {
	b.restarts++
	return b.restartErr
}

type fakeAudit struct{ actions []string }

func (a *fakeAudit) Record(_ context.Context, action, app, key, actor string) {
	a.actions = append(a.actions, action+":"+app+"/"+key+":"+actor)
}

func newRig(p store.SecretRotationPolicy) (*Engine, *fakeStore, *fakeBackend, *fakeAudit) {
	st := &fakeStore{policies: map[string]store.SecretRotationPolicy{p.ServiceName + "/" + p.EnvKey: p}}
	be := &fakeBackend{
		apps:    map[string]bool{p.ServiceName: false},
		values:  map[string]string{p.ServiceName + "/" + p.EnvKey: "old-value"},
		locked:  map[string]bool{},
		updated: map[string]time.Time{p.ServiceName + "/" + p.EnvKey: t0.AddDate(0, 0, -40)},
		setErr:  map[string]error{},
	}
	au := &fakeAudit{}
	return &Engine{Store: st, Backend: be, Audit: au, Now: func() time.Time { return t0 }}, st, be, au
}

func basePolicy() store.SecretRotationPolicy {
	return store.SecretRotationPolicy{ServiceName: "web", EnvKey: "JWT_SECRET", IntervalDays: 30, GraceHours: 24, AutoRotate: true, State: store.SecretRotationIdle}
}

func TestRotateHappyPathKeepsPreviousAndAudits(t *testing.T) {
	e, st, be, au := newRig(basePolicy())
	if err := e.Rotate(context.Background(), "web", "JWT_SECRET", "alice"); err != nil {
		t.Fatal(err)
	}
	if be.values["web/JWT_SECRET"] == "old-value" || len(be.values["web/JWT_SECRET"]) != 64 {
		t.Fatalf("value not replaced with a generated one")
	}
	if be.values["web/JWT_SECRET_PREVIOUS"] != "old-value" {
		t.Fatal("previous value not kept for the grace window")
	}
	p := st.policies["web/JWT_SECRET"]
	if p.State != store.SecretRotationIdle || !p.LastRotatedAt.Equal(t0) || !p.PreviousExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("progress wrong: %+v", p)
	}
	if be.restarts != 1 || len(au.actions) != 1 || au.actions[0] != "secret.rotate:web/JWT_SECRET:alice" {
		t.Fatalf("restarts=%d audit=%v", be.restarts, au.actions)
	}
}

func TestRotateRestartFailureIsRecoverableAndVisible(t *testing.T) {
	e, st, be, au := newRig(basePolicy())
	be.restartErr = errors.New("docker unreachable token=abc")
	err := e.Rotate(context.Background(), "web", "JWT_SECRET", "system")
	if err == nil {
		t.Fatal("expected restart failure")
	}
	p := st.policies["web/JWT_SECRET"]
	if p.State != store.SecretRotationRestartFailed || p.LastError != ReasonRestartFailed {
		t.Fatalf("state = %s / %q", p.State, p.LastError)
	}
	if strings.Contains(p.LastError, "token") {
		t.Fatal("underlying error leaked into persisted state")
	}
	newValue := be.values["web/JWT_SECRET"]
	if newValue == "old-value" || be.values["web/JWT_SECRET_PREVIOUS"] != "old-value" {
		t.Fatal("new value must be stored with the previous kept")
	}
	if got := au.actions[len(au.actions)-1]; !strings.HasPrefix(got, "secret.rotate.restart_failed") {
		t.Fatalf("restart failure not audited: %v", au.actions)
	}

	be.restartErr = nil
	e.Sweep(context.Background())
	p = st.policies["web/JWT_SECRET"]
	if p.State != store.SecretRotationIdle || p.LastError != "" {
		t.Fatalf("not recovered: %+v", p)
	}
	if be.values["web/JWT_SECRET"] != newValue {
		t.Fatal("recovery must retry the restart, not regenerate the value")
	}
}

func TestRotateFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*fakeBackend)
		reason  string
		wantOld bool
	}{
		{"locked", func(b *fakeBackend) { b.locked["web/JWT_SECRET"] = true }, ReasonLocked, true},
		{"missing", func(b *fakeBackend) { delete(b.values, "web/JWT_SECRET") }, ReasonMissing, false},
		{"grace key owned by someone else", func(b *fakeBackend) { b.values["web/JWT_SECRET_PREVIOUS"] = "mine" }, ReasonPreviousInUse, true},
		{"previous write fails", func(b *fakeBackend) { b.setErr["JWT_SECRET_PREVIOUS"] = errors.New("disk") }, ReasonWriteFailed, true},
		{"new write fails", func(b *fakeBackend) { b.setErr["JWT_SECRET"] = errors.New("disk") }, ReasonWriteFailed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, st, be, _ := newRig(basePolicy())
			tt.mutate(be)
			if err := e.Rotate(context.Background(), "web", "JWT_SECRET", "system"); err == nil {
				t.Fatal("expected failure")
			}
			p := st.policies["web/JWT_SECRET"]
			if p.State != store.SecretRotationFailed || p.LastError != tt.reason {
				t.Fatalf("state=%s reason=%q", p.State, p.LastError)
			}
			if tt.wantOld && be.values["web/JWT_SECRET"] != "old-value" {
				t.Fatal("value changed on a failed rotation")
			}
			if be.restarts != 0 {
				t.Fatal("must not restart after a failed rotation")
			}
			if tt.name == "new write fails" {
				if _, ok := be.values["web/JWT_SECRET_PREVIOUS"]; ok {
					t.Fatal("orphan previous value left behind")
				}
			}
		})
	}
}

func TestRotateRefusesNonAuto(t *testing.T) {
	p := basePolicy()
	p.AutoRotate = false
	e, _, be, _ := newRig(p)
	if err := e.Rotate(context.Background(), "web", "JWT_SECRET", "x"); !errors.Is(err, ErrNotAuto) {
		t.Fatalf("err = %v", err)
	}
	if be.values["web/JWT_SECRET"] != "old-value" {
		t.Fatal("value changed")
	}
}

func TestSweepScheduling(t *testing.T) {
	tests := []struct {
		name      string
		age       int
		auto      bool
		suspended bool
		wantRot   int
		restarts  int
	}{
		{"not due", 10, true, false, 0, 0},
		{"due", 31, true, false, 1, 1},
		{"due but not auto", 31, false, false, 0, 0},
		{"due, app stopped, no restart", 31, true, true, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := basePolicy()
			p.AutoRotate = tt.auto
			e, _, be, _ := newRig(p)
			be.updated["web/JWT_SECRET"] = t0.AddDate(0, 0, -tt.age)
			be.apps["web"] = tt.suspended
			if got := e.Sweep(context.Background()); got != tt.wantRot {
				t.Fatalf("rotated = %d, want %d", got, tt.wantRot)
			}
			if be.restarts != tt.restarts {
				t.Fatalf("restarts = %d, want %d", be.restarts, tt.restarts)
			}
		})
	}
}

func TestSweepExpiresGraceAndDropsDeletedApps(t *testing.T) {
	p := basePolicy()
	p.PreviousExpiresAt = t0.Add(-time.Minute)
	e, st, be, au := newRig(p)
	be.values["web/JWT_SECRET_PREVIOUS"] = "old"
	be.updated["web/JWT_SECRET"] = t0
	e.Sweep(context.Background())
	if _, ok := be.values["web/JWT_SECRET_PREVIOUS"]; ok {
		t.Fatal("previous value survived its grace window")
	}
	if !st.policies["web/JWT_SECRET"].PreviousExpiresAt.IsZero() {
		t.Fatal("expiry not cleared")
	}
	if len(au.actions) != 1 || !strings.HasPrefix(au.actions[0], "secret.rotate.grace_expired") {
		t.Fatalf("audit = %v", au.actions)
	}

	delete(be.apps, "web")
	e.Sweep(context.Background())
	if len(st.policies) != 0 {
		t.Fatal("policy of a deleted app not dropped")
	}
}

func TestSweepDoesNotRetryFailedState(t *testing.T) {
	p := basePolicy()
	p.State = store.SecretRotationFailed
	p.LastError = ReasonLocked
	e, _, be, _ := newRig(p)
	be.updated["web/JWT_SECRET"] = t0.AddDate(0, 0, -90)
	if got := e.Sweep(context.Background()); got != 0 {
		t.Fatalf("rotated %d from failed state", got)
	}
}

func TestRotateValueSetsSuppliedValueWithoutRestartAndExpiresGrace(t *testing.T) {
	p := basePolicy()
	p.AutoRotate = false
	e, st, be, au := newRig(p)
	if err := e.RotateValue(context.Background(), "web", "JWT_SECRET", "supplied-next", "alice"); err != nil {
		t.Fatal(err)
	}
	if be.values["web/JWT_SECRET"] != "supplied-next" || be.values["web/JWT_SECRET_PREVIOUS"] != "old-value" {
		t.Fatal("supplied value not stored with the previous kept")
	}
	if be.restarts != 0 {
		t.Fatalf("restarts = %d, want 0 so the caller can offer a redeploy", be.restarts)
	}
	got := st.policies["web/JWT_SECRET"]
	if got.State != store.SecretRotationIdle || !got.PreviousExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("progress wrong: %+v", got)
	}
	if len(au.actions) != 1 || au.actions[0] != "secret.rotate:web/JWT_SECRET:alice" {
		t.Fatalf("audit = %v", au.actions)
	}

	e.Now = func() time.Time { return t0.Add(23 * time.Hour) }
	e.Sweep(context.Background())
	if _, ok := be.values["web/JWT_SECRET_PREVIOUS"]; !ok {
		t.Fatal("previous value dropped inside the grace window")
	}
	e.Now = func() time.Time { return t0.Add(25 * time.Hour) }
	e.Sweep(context.Background())
	if _, ok := be.values["web/JWT_SECRET_PREVIOUS"]; ok {
		t.Fatal("previous value kept past the grace window")
	}
	if last := au.actions[len(au.actions)-1]; !strings.HasPrefix(last, ActionGraceExpired) {
		t.Fatalf("grace expiry not audited: %v", au.actions)
	}
}

func TestRotateValueGeneratesWhenEmpty(t *testing.T) {
	p := basePolicy()
	p.AutoRotate = false
	e, _, be, _ := newRig(p)
	if err := e.RotateValue(context.Background(), "web", "JWT_SECRET", "", "alice"); err != nil {
		t.Fatal(err)
	}
	if v := be.values["web/JWT_SECRET"]; v == "old-value" || len(v) != 64 {
		t.Fatal("expected a generated value")
	}
}
