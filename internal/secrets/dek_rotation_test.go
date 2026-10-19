package secrets

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type dekFakeStore struct {
	*fakeStore
	created map[string]time.Time
	rotated map[string]time.Time
	runs    []store.DEKRotationRun
	// afterRotate runs after each committed service rotation.
	afterRotate func(calls int)
	calls       int
	// failService makes that service's rewrite see a changed row.
	failService string
}

func newDEKFakeStore() *dekFakeStore {
	return &dekFakeStore{fakeStore: newFakeStore(), created: map[string]time.Time{}, rotated: map[string]time.Time{}}
}

func (f *dekFakeStore) SaveServiceDEK(ctx context.Context, service string, wrapped []byte) error {
	f.created[service] = time.Now().UTC().Add(-48 * time.Hour)
	return f.fakeStore.SaveServiceDEK(ctx, service, wrapped)
}

func (f *dekFakeStore) ListServiceDEKStatus(context.Context) ([]store.ServiceDEKStatus, error) {
	var out []store.ServiceDEKStatus
	for name := range f.deks {
		out = append(out, store.ServiceDEKStatus{ServiceName: name, CreatedAt: f.created[name], RotatedAt: f.rotated[name], ValueCount: len(f.values[name])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServiceName < out[j].ServiceName })
	return out, nil
}

func (f *dekFakeStore) RotateServiceDEK(_ context.Context, service string, at time.Time,
	rewrite func([]byte, []store.SecretCiphertext) ([]byte, [][]byte, error)) (int, error) {
	wrapped, ok := f.deks[service]
	if !ok {
		return 0, store.ErrServiceDEKNotFound
	}
	var rows []store.SecretCiphertext
	for k, ct := range f.values[service] {
		rows = append(rows, store.SecretCiphertext{ServiceName: service, EnvKey: k, Ciphertext: ct})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EnvKey < rows[j].EnvKey })
	newWrapped, repl, err := rewrite(wrapped, rows)
	if err != nil {
		return 0, err
	}
	if service == f.failService {
		return 0, store.ErrDEKRotationConflict
	}
	f.deks[service] = newWrapped
	for i, row := range rows {
		f.values[service][row.EnvKey] = repl[i]
	}
	f.rotated[service] = at
	f.calls++
	if f.afterRotate != nil {
		f.afterRotate(f.calls)
	}
	return len(rows), nil
}

func (f *dekFakeStore) LatestDEKRotationRun(context.Context) (store.DEKRotationRun, bool, error) {
	if len(f.runs) == 0 {
		return store.DEKRotationRun{}, false, nil
	}
	return f.runs[len(f.runs)-1], true, nil
}

func (f *dekFakeStore) CreateDEKRotationRun(_ context.Context, r store.DEKRotationRun) error {
	f.runs = append(f.runs, r)
	return nil
}

func (f *dekFakeStore) UpdateDEKRotationRun(_ context.Context, r store.DEKRotationRun) error {
	for i := range f.runs {
		if f.runs[i].ID == r.ID {
			f.runs[i] = r
		}
	}
	return nil
}

func dekTestSetup(t *testing.T) (*Manager, *dekFakeStore, *DEKRotator) {
	t.Helper()
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	fs := newDEKFakeStore()
	m := NewManager(fs, mk)
	ctx := context.Background()
	for _, svc := range []string{"web", "worker", "api"} {
		for _, key := range []string{"A", "B"} {
			if err := m.SetValue(ctx, svc, key, svc+"-"+key+"-plain"); err != nil {
				t.Fatalf("SetValue() error = %v", err)
			}
		}
	}
	return m, fs, NewDEKRotator(m, fs, nil)
}

func requireReadable(t *testing.T, m *Manager) {
	t.Helper()
	for _, svc := range []string{"web", "worker", "api"} {
		for _, key := range []string{"A", "B"} {
			got, err := m.Resolve(context.Background(), svc, key)
			if err != nil || got != svc+"-"+key+"-plain" {
				t.Fatalf("Resolve(%s,%s) = %q, %v", svc, key, got, err)
			}
		}
	}
}

func TestRotateOne_KeepsEverySecretReadable(t *testing.T) {
	m, fs, r := dekTestSetup(t)
	oldWrapped := bytes.Clone(fs.deks["web"])
	oldCT := bytes.Clone(fs.values["web"]["A"])
	otherWrapped := bytes.Clone(fs.deks["api"])

	res, err := r.RotateOne(context.Background(), "web")
	if err != nil || res.Values != 2 {
		t.Fatalf("RotateOne() = %+v, %v", res, err)
	}
	if bytes.Equal(oldWrapped, fs.deks["web"]) || bytes.Equal(oldCT, fs.values["web"]["A"]) {
		t.Fatal("DEK or ciphertext was not replaced")
	}
	if !bytes.Equal(otherWrapped, fs.deks["api"]) {
		t.Fatal("rotating web touched api's DEK")
	}
	requireReadable(t, m)
	if fs.rotated["web"].IsZero() {
		t.Fatal("rotated_at not recorded")
	}
}

func TestRotateOne_MissingService(t *testing.T) {
	_, _, r := dekTestSetup(t)
	if _, err := r.RotateOne(context.Background(), "ghost"); !errors.Is(err, store.ErrServiceDEKNotFound) {
		t.Fatalf("err = %v, want ErrServiceDEKNotFound", err)
	}
}

func TestRotateOne_WrongKeyDetectedNothingChanges(t *testing.T) {
	m, fs, _ := dekTestSetup(t)
	other, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	r := NewDEKRotator(NewManager(fs, other), fs, nil)
	before := bytes.Clone(fs.deks["web"])
	ctBefore := bytes.Clone(fs.values["web"]["A"])

	if _, err := r.RotateOne(context.Background(), "web"); !errors.Is(err, ErrDEKUnwrap) {
		t.Fatalf("err = %v, want ErrDEKUnwrap", err)
	}
	if !bytes.Equal(before, fs.deks["web"]) || !bytes.Equal(ctBefore, fs.values["web"]["A"]) {
		t.Fatal("state changed after a failed rotation")
	}
	requireReadable(t, m)
}

func TestRotateOne_CorruptValueAbortsWholeService(t *testing.T) {
	m, fs, r := dekTestSetup(t)
	fs.values["web"]["B"] = []byte("not a ciphertext")
	wrapped := bytes.Clone(fs.deks["web"])
	ctA := bytes.Clone(fs.values["web"]["A"])

	if _, err := r.RotateOne(context.Background(), "web"); err == nil {
		t.Fatal("expected an error for an undecryptable value")
	}
	if !bytes.Equal(wrapped, fs.deks["web"]) || !bytes.Equal(ctA, fs.values["web"]["A"]) {
		t.Fatal("partial rotation leaked out of a failed service")
	}
	if got, err := m.Resolve(context.Background(), "web", "A"); err != nil || got != "web-A-plain" {
		t.Fatalf("Resolve() = %q, %v", got, err)
	}
}

func TestRotateAll_RotatesEveryServiceAndIsIdempotentPerRun(t *testing.T) {
	m, fs, r := dekTestSetup(t)
	res, err := r.RotateAll(context.Background(), "root")
	if err != nil || res.Rotated != 3 || !res.Complete || res.FailedCount != 0 {
		t.Fatalf("RotateAll() = %+v, %v", res, err)
	}
	requireReadable(t, m)
	if len(fs.runs) != 1 || !fs.runs[0].Finished() {
		t.Fatalf("runs = %+v", fs.runs)
	}

	again, err := r.RotateAll(context.Background(), "root")
	if err != nil || again.Resumed || again.Rotated != 3 {
		t.Fatalf("second RotateAll() = %+v, %v (a finished run must not be reused)", again, err)
	}
	requireReadable(t, m)
}

func TestRotateAll_InterruptedRunResumesWithoutRedoingFinishedServices(t *testing.T) {
	m, fs, r := dekTestSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	fs.afterRotate = func(calls int) {
		if calls == 1 {
			cancel()
		}
	}
	first, err := r.RotateAll(ctx, "root")
	if err != nil || first.Complete || first.Rotated != 1 {
		t.Fatalf("interrupted RotateAll() = %+v, %v", first, err)
	}
	if fs.runs[0].Finished() {
		t.Fatal("an interrupted run must stay open")
	}
	requireReadable(t, m)

	fs.afterRotate = nil
	resumedAt := fs.calls
	second, err := r.RotateAll(context.Background(), "root")
	if err != nil || !second.Resumed || !second.Complete || second.Rotated != 2 || second.Skipped != 1 {
		t.Fatalf("resumed RotateAll() = %+v, %v", second, err)
	}
	if fs.calls-resumedAt != 2 {
		t.Fatalf("rotated %d services on resume, want 2", fs.calls-resumedAt)
	}
	requireReadable(t, m)
}

func TestRotateAll_OneFailingServiceDoesNotBlockTheRest(t *testing.T) {
	m, fs, r := dekTestSetup(t)
	fs.failService = "worker"
	res, err := r.RotateAll(context.Background(), "root")
	if err != nil || res.Rotated != 2 || res.FailedCount != 1 || res.Failed[0].Service != "worker" {
		t.Fatalf("RotateAll() = %+v, %v", res, err)
	}
	requireReadable(t, m)
}

func TestRotateAll_BusyRejectsSecondCaller(t *testing.T) {
	_, fs, r := dekTestSetup(t)
	inner := make(chan struct{})
	release := make(chan struct{})
	fs.afterRotate = func(int) {
		close(inner)
		<-release
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.RotateOne(context.Background(), "web")
		done <- err
	}()
	<-inner
	if _, err := r.RotateOne(context.Background(), "api"); !errors.Is(err, ErrDEKRotationBusy) {
		t.Fatalf("err = %v, want ErrDEKRotationBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDEKStatus_ReportsAgeAndLastRun(t *testing.T) {
	_, _, r := dekTestSetup(t)
	st, err := r.Status(context.Background())
	if err != nil || len(st.Services) != 3 || st.LastRun != nil {
		t.Fatalf("Status() = %+v, %v", st, err)
	}
	for _, s := range st.Services {
		if s.AgeDays < 1 || s.ValueCount != 2 {
			t.Fatalf("service %+v", s)
		}
	}
	if _, err := r.RotateAll(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	st, _ = r.Status(context.Background())
	if st.LastRun == nil || st.Services[0].AgeDays != 0 {
		t.Fatalf("Status() after rotation = %+v", st)
	}
}
