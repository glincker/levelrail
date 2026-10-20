package ingress

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	simulatedOutage = 10 * time.Minute
	simulatedTick   = 30 * time.Second
)

// downCertStore is a certificate store whose database is unavailable while down is set.
type downCertStore struct {
	mu   sync.Mutex
	down bool
	data map[string][]byte
}

var errStoreDown = errors.New("database is unavailable")

func newDownCertStore() *downCertStore { return &downCertStore{data: map[string][]byte{}} }

func (d *downCertStore) setDown(v bool) { d.mu.Lock(); d.down = v; d.mu.Unlock() }

func (d *downCertStore) check() error {
	if d.down {
		return errStoreDown
	}
	return nil
}

func (d *downCertStore) SaveCertStorageValue(_ context.Context, key string, value []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(); err != nil {
		return err
	}
	d.data[key] = value
	return nil
}

func (d *downCertStore) GetCertStorageValue(_ context.Context, key string) (*store.CertStorageValue, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(); err != nil {
		return nil, err
	}
	v, ok := d.data[key]
	if !ok {
		return nil, store.ErrCertStorageKeyNotFound
	}
	return &store.CertStorageValue{Value: v}, nil
}

func (d *downCertStore) DeleteCertStorageValue(_ context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(); err != nil {
		return err
	}
	delete(d.data, key)
	return nil
}

func (d *downCertStore) ExistsCertStorageValue(_ context.Context, key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(); err != nil {
		return false, err
	}
	_, ok := d.data[key]
	return ok, nil
}

func (d *downCertStore) ListCertStorageKeys(context.Context, string, bool) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (d *downCertStore) StatCertStorageValue(context.Context, string) (*store.CertStorageKeyInfo, error) {
	return nil, errStoreDown
}

func (d *downCertStore) AcquireCertStorageLock(context.Context, string, time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.check() == nil, d.check()
}

func (d *downCertStore) TouchCertStorageLock(context.Context, string) error { return d.check() }
func (d *downCertStore) ReleaseCertStorageLock(context.Context, string) error {
	return d.check()
}

// TestCertStorageOutageIsRetriableNotMissing covers TLS renewals during a control plane outage:
// a renewal that cannot be saved fails loudly so certmagic retries it on its next maintenance
// pass, and a database that is down never reads as "certificate missing", which would trigger a
// needless re-issue the moment it came back. Writes land once the database returns.
func TestCertStorageOutageIsRetriableNotMissing(t *testing.T) {
	ctx := context.Background()
	cs := newDownCertStore()
	st := NewSQLiteStorage(cs, testLogger(t))
	const key = "certificates/acme/example.com/example.com.crt"

	if err := st.Store(ctx, key, []byte("v1")); err != nil {
		t.Fatalf("store before the outage: %v", err)
	}
	cs.setDown(true)

	attempts := 0
	for elapsed := time.Duration(0); elapsed < simulatedOutage; elapsed += simulatedTick {
		attempts++
		if err := st.Store(ctx, key, []byte("v2")); err == nil {
			t.Fatalf("tick %d: a renewal must fail while the database is down, not be silently dropped", attempts)
		}
		if _, err := st.Load(ctx, key); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("tick %d: Load during the outage must be a hard error, got %v", attempts, err)
		}
		if _, err := st.List(ctx, "certificates", true); err == nil || errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("tick %d: List during the outage must be a hard error, got %v", attempts, err)
		}
	}
	if attempts != int(simulatedOutage/simulatedTick) {
		t.Fatalf("simulated %d ticks, want %d", attempts, int(simulatedOutage/simulatedTick))
	}

	cs.setDown(false)
	if got, err := st.Load(ctx, key); err != nil || string(got) != "v1" {
		t.Fatalf("the certificate stored before the outage must still be there: %q %v", got, err)
	}
	if err := st.Store(ctx, key, []byte("v2")); err != nil {
		t.Fatalf("a retried renewal must succeed after recovery: %v", err)
	}
	if err := st.Lock(ctx, "renew-example.com"); err != nil {
		t.Fatalf("the renewal lock must work again: %v", err)
	}
	if err := st.Unlock(ctx, "renew-example.com"); err != nil {
		t.Fatal(err)
	}
}

// TestSnapshotRestoreNeedsNoDatabase proves the restart half: after the process was down for the
// whole simulated outage, the route table comes back from the sealed snapshot on disk even though
// the database is not available yet.
func TestSnapshotRestoreNeedsNoDatabase(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	loads := 0
	loadConfig = func([]byte, bool) error { loads++; return nil }

	snap := newTestSnapshot(t)
	before := New(nil)
	before.SetSnapshot(snap, "bound")
	cfg := &Config{Admin: &AdminConfig{Listen: "127.0.0.1:2999"}, Apps: Apps{}}
	if err := before.Apply(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	cs := newDownCertStore()
	cs.setDown(true)
	SetActiveCertStorage(NewSQLiteStorage(cs, testLogger(t)))
	t.Cleanup(func() { SetActiveCertStorage(nil) })

	loads = 0
	after := New(nil)
	after.SetSnapshot(snap, "bound")
	ok, err := after.RestoreSnapshot(context.Background())
	if err != nil || !ok || loads != 1 {
		t.Fatalf("restore with the database down: ok=%v err=%v loads=%d", ok, err, loads)
	}
}

// TestDriverKeepsRoutingThroughStalledControlPlane runs a real embedded Caddy. The control plane
// side keeps trying to apply routes for 10 simulated minutes and every attempt fails; the route
// table applied before the outage must keep answering, and the same config applies again once
// the control plane recovers.
func TestDriverKeepsRoutingThroughStalledControlPlane(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "still serving")
	}))
	defer backend.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	good, err := BuildProxyConfig(ProxyOptions{ServerName: "outage-http", ListenAddr: addr, BackendDial: backend.Listener.Addr().String(), StorageDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	d := New(testLogger(t))
	if err := d.Apply(ctx, good); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Stop(ctx) }()
	if got := getBodyWithRetry(t, "http://"+addr+"/"); got != "still serving" {
		t.Fatalf("baseline response %q", got)
	}

	squatter, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = squatter.Close() }()
	broken, err := BuildProxyConfig(ProxyOptions{ServerName: "outage-http-2", ListenAddr: squatter.Addr().String(), BackendDial: backend.Listener.Addr().String(), StorageDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	for elapsed := time.Duration(0); elapsed < simulatedOutage; elapsed += simulatedTick {
		if err := d.Apply(ctx, broken); err == nil {
			t.Fatal("applying a config whose listener is taken must fail")
		}
		body, status, err := doGet(http.DefaultClient, "http://"+addr+"/")
		if err != nil || status != http.StatusOK || body != "still serving" {
			t.Fatalf("at +%s the last applied routes stopped answering: %q %d %v", elapsed, body, status, err)
		}
	}

	if err := d.Apply(ctx, good); err != nil {
		t.Fatalf("the original config must apply again after the outage: %v", err)
	}
	if got := getBodyWithRetry(t, "http://"+addr+"/"); got != "still serving" {
		t.Fatalf("after recovery response %q", got)
	}
}
