package secrets

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func seedLegacy(t *testing.T, m *Manager, fs *fakeStore, serviceName, envKey, plaintext string) {
	t.Helper()
	dek, err := m.dekFor(context.Background(), serviceName)
	if err != nil {
		t.Fatalf("dekFor(%q) error = %v", serviceName, err)
	}
	if err := fs.SaveSecretValue(context.Background(), serviceName, envKey, encryptLegacy(t, dek, plaintext)); err != nil {
		t.Fatal(err)
	}
}

func mustResolve(t *testing.T, m *Manager, serviceName, envKey, want string) {
	t.Helper()
	got, err := m.Resolve(context.Background(), serviceName, envKey)
	if err != nil {
		t.Fatalf("Resolve(%q, %q) error = %v", serviceName, envKey, err)
	}
	if got != want {
		t.Fatalf("Resolve(%q, %q) = %q, want %q", serviceName, envKey, got, want)
	}
}

func TestManager_SwappedCiphertextFailsClosed(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		// move copies what an attacker with DB write access would copy.
		move func(fs *fakeStore)
		svc  string
		key  string
	}{
		{
			name: "same service, other key",
			move: func(fs *fakeStore) { fs.values["web"]["API_KEY"] = fs.values["web"]["DATABASE_URL"] },
			svc:  "web", key: "API_KEY",
		},
		{
			name: "other service, DEK copied along",
			move: func(fs *fakeStore) {
				fs.deks["api"] = fs.deks["web"]
				fs.values["api"] = map[string][]byte{"DATABASE_URL": fs.values["web"]["DATABASE_URL"]}
			},
			svc: "api", key: "DATABASE_URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, fs := testManager(t)
			if err := m.SetValue(ctx, "web", "DATABASE_URL", "postgres://secret"); err != nil {
				t.Fatal(err)
			}
			if err := m.SetValue(ctx, "web", "API_KEY", "sk-real"); err != nil {
				t.Fatal(err)
			}
			tt.move(fs)

			got, err := m.Resolve(ctx, tt.svc, tt.key)
			if !errors.Is(err, ErrBindingMismatch) {
				t.Fatalf("Resolve() = (%q, %v), want ErrBindingMismatch", got, err)
			}
			mustResolve(t, m, "web", "DATABASE_URL", "postgres://secret")
		})
	}
}

type recordHandler struct {
	records []slog.Record
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordHandler) WithGroup(string) slog.Handler      { return h }

func TestManager_LegacyValueResolvesAndLogsOnceWithoutValues(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	h := &recordHandler{}
	fs := newFakeStore()
	m := NewManager(fs, mk, WithLogger(slog.New(h)))
	seedLegacy(t, m, fs, "tenant-owner", "API_KEY", "legacy-plaintext")

	for range 3 {
		mustResolve(t, m, "tenant-owner", "API_KEY", "legacy-plaintext")
	}
	if len(h.records) != 1 {
		t.Fatalf("got %d log records for 3 legacy reads, want 1", len(h.records))
	}
	r := h.records[0]
	if r.Level != slog.LevelDebug {
		t.Errorf("legacy notice level = %v, want debug", r.Level)
	}
	var logged strings.Builder
	logged.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		logged.WriteString(" " + a.String())
		return true
	})
	for _, forbidden := range []string{"legacy-plaintext", "tenant-owner"} {
		if strings.Contains(logged.String(), forbidden) {
			t.Errorf("legacy notice %q leaks %q", logged.String(), forbidden)
		}
	}

	status, err := m.BindingStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != (BindingStatus{Total: 1, Bound: 0, Legacy: 1}) {
		t.Errorf("BindingStatus() = %+v, want 1 legacy", status)
	}
}

func TestManager_RequireBoundRejectsLegacy(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	fs := newFakeStore()
	m := NewManager(fs, mk, WithRequireBound(true))
	seedLegacy(t, m, fs, "web", "OLD", "v")
	if _, err := m.Resolve(context.Background(), "web", "OLD"); !errors.Is(err, ErrLegacyRejected) {
		t.Fatalf("Resolve(legacy) error = %v, want ErrLegacyRejected", err)
	}

	if _, err := m.Rebind(context.Background()); err != nil {
		t.Fatalf("Rebind() error = %v", err)
	}
	mustResolve(t, m, "web", "OLD", "v")
}

func seedMixed(t *testing.T, m *Manager, fs *fakeStore) map[[2]string]string {
	t.Helper()
	want := map[[2]string]string{}
	for _, svc := range []string{"api", "backup-target/b1", "web"} {
		for _, key := range []string{"A", "B", "C"} {
			v := svc + "-" + key + "-value"
			seedLegacy(t, m, fs, svc, key, v)
			want[[2]string{svc, key}] = v
		}
	}
	if err := m.SetValue(context.Background(), "web", "BOUND", "already"); err != nil {
		t.Fatal(err)
	}
	want[[2]string{"web", "BOUND"}] = "already"
	return want
}

func TestManager_RebindIsIdempotent(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	want := seedMixed(t, m, fs)

	first, err := m.Rebind(ctx)
	if err != nil {
		t.Fatalf("Rebind() error = %v", err)
	}
	if first.Scanned != 10 || first.Rebound != 9 || first.AlreadyBound != 1 || first.Remaining != 0 || first.FailedCount != 0 {
		t.Fatalf("first Rebind() = %+v, want 9 rebound, 1 already bound, 0 remaining", first)
	}
	for slot, v := range want {
		mustResolve(t, m, slot[0], slot[1], v)
		if !HasBoundPrefix(fs.values[slot[0]][slot[1]]) {
			t.Errorf("%v is not bound after Rebind", slot)
		}
	}

	snapshot := map[[2]string][]byte{}
	for slot := range want {
		snapshot[slot] = bytes.Clone(fs.values[slot[0]][slot[1]])
	}
	second, err := m.Rebind(ctx)
	if err != nil {
		t.Fatalf("second Rebind() error = %v", err)
	}
	if second.Rebound != 0 || second.AlreadyBound != 10 || second.Remaining != 0 {
		t.Fatalf("second Rebind() = %+v, want a no-op", second)
	}
	for slot, ct := range snapshot {
		if !bytes.Equal(fs.values[slot[0]][slot[1]], ct) {
			t.Errorf("second Rebind rewrote %v", slot)
		}
	}
}

func TestManager_RebindResumesAfterMidRunFailure(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	want := seedMixed(t, m, fs)

	fs.failReplaceAfter = 4
	partial, err := m.Rebind(ctx)
	if err == nil {
		t.Fatal("Rebind() with an injected store failure error = nil, want an error")
	}
	if partial.Rebound != 4 {
		t.Fatalf("partial Rebind() rebound = %d, want 4 before the failure", partial.Rebound)
	}
	status, err := m.BindingStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Legacy != 5 {
		t.Fatalf("legacy after half-succeeded run = %d, want 5", status.Legacy)
	}
	for slot, v := range want {
		mustResolve(t, m, slot[0], slot[1], v)
	}

	fs.failReplaceAfter = 0
	resumed, err := m.Rebind(ctx)
	if err != nil {
		t.Fatalf("resumed Rebind() error = %v", err)
	}
	if resumed.Rebound != 5 || resumed.AlreadyBound != 5 || resumed.Remaining != 0 {
		t.Fatalf("resumed Rebind() = %+v, want the remaining 5 rebound", resumed)
	}
	for slot, v := range want {
		mustResolve(t, m, slot[0], slot[1], v)
	}
}

func TestManager_RebindReportsSwappedBoundValueAndLeavesIt(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	if err := m.SetValue(ctx, "web", "A", "a"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetValue(ctx, "web", "B", "b"); err != nil {
		t.Fatal(err)
	}
	fs.values["web"]["B"] = fs.values["web"]["A"]
	fs.values["orphan"] = map[string][]byte{"X": []byte("no dek")}

	res, err := m.Rebind(ctx)
	if err != nil {
		t.Fatalf("Rebind() error = %v", err)
	}
	if res.FailedCount != 2 || len(res.Failed) != 2 {
		t.Fatalf("Rebind() = %+v, want 2 failures", res)
	}
	reasons := res.Failed[0].Reason + "|" + res.Failed[1].Reason
	if !strings.Contains(reasons, "different slot") || !strings.Contains(reasons, "no data encryption key") {
		t.Errorf("failure reasons = %q", reasons)
	}
	if !bytes.Equal(fs.values["web"]["B"], fs.values["web"]["A"]) {
		t.Error("Rebind rewrote a mismatched value, it must never launder a swap")
	}
}

type racingStore struct {
	*fakeStore
	m *Manager
}

func (r *racingStore) ReplaceSecretCiphertext(ctx context.Context, serviceName, envKey string, old, replacement []byte) (bool, error) {
	if err := r.m.SetValue(ctx, serviceName, envKey, "written-mid-run"); err != nil {
		return false, err
	}
	return r.fakeStore.ReplaceSecretCiphertext(ctx, serviceName, envKey, old, replacement)
}

func TestManager_RebindNeverClobbersConcurrentWrite(t *testing.T) {
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	fs := newFakeStore()
	seeder := NewManager(fs, mk)
	seedLegacy(t, seeder, fs, "web", "K", "old")

	rs := &racingStore{fakeStore: fs}
	m := NewManager(rs, mk)
	rs.m = seeder

	res, err := m.Rebind(context.Background())
	if err != nil {
		t.Fatalf("Rebind() error = %v", err)
	}
	if res.Changed != 1 || res.Rebound != 0 {
		t.Fatalf("Rebind() = %+v, want 1 changed", res)
	}
	mustResolve(t, m, "web", "K", "written-mid-run")
}

func TestManager_RotationThenRebind(t *testing.T) {
	ctx := context.Background()
	m, fs := testManager(t)
	want := seedMixed(t, m, fs)

	newKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.RotateMasterKey(ctx, newKey.String()); err != nil {
		t.Fatalf("RotateMasterKey() error = %v", err)
	}
	for slot, v := range want {
		mustResolve(t, m, slot[0], slot[1], v)
	}
	res, err := m.Rebind(ctx)
	if err != nil || res.Rebound != 9 || res.Remaining != 0 {
		t.Fatalf("Rebind() after rotation = (%+v, %v)", res, err)
	}
	for slot, v := range want {
		mustResolve(t, m, slot[0], slot[1], v)
	}
}
