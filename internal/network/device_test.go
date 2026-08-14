package network

// Device's tests run against fakes for the two things that need root: the
// TUN device and the wireguard-go device. That is not a shortcut around
// testing the real thing, it is the only way to test the parts of this
// type that are not root-dependent (the read-diff-write cycle, the
// close-on-failed-startup path, the missing-link-configurator warning,
// backend reporting), all of which are where its bugs would live.
//
// What these tests deliberately do not prove: that wireguard-go actually
// encrypts and routes packets. That needs two real hosts and root, and is
// a live test against a real Linux node, not a unit test.

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTUN struct {
	name     string
	nameErr  error
	closed   bool
	closeErr error
}

func (f *fakeTUN) Name() (string, error) { return f.name, f.nameErr }
func (f *fakeTUN) Close() error {
	f.closed = true
	return f.closeErr
}

type fakeWG struct {
	mu sync.Mutex

	getResponse string
	getErr      error
	setErr      error
	upErr       error

	sets   []string
	closed bool
}

func (f *fakeWG) IpcSet(conf string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.sets = append(f.sets, conf)
	return nil
}

func (f *fakeWG) IpcGet() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getResponse, f.getErr
}

func (f *fakeWG) Up() error { return f.upErr }

func (f *fakeWG) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

func (f *fakeWG) lastSet(t *testing.T) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sets) == 0 {
		t.Fatal("no config was ever applied to the device")
	}
	return f.sets[len(f.sets)-1]
}

// withFakes injects the TUN and wireguard-go constructors. Available only
// inside this package, which is why DeviceOption's parameter type is
// unexported.
func withFakes(tunDev *fakeTUN, wg *fakeWG, backend Backend) DeviceOption {
	return func(o *deviceOptions) {
		o.newTUN = func(string, int) (tunDevice, error) { return tunDev, nil }
		o.newWG = func(tunDevice, *slog.Logger, string) wgDevice { return wg }
		o.backend = backend
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func privilegedProbe() Probe { return fakeProbe{os: "linux", privileged: true} }

func TestNewDevice_DisabledWhenUnprivileged(t *testing.T) {
	m, err := NewDevice(context.Background(), fakeProbe{os: "linux", privileged: false},
		WithLogger(quietLogger()), WithShortName("acme"))
	if err != nil {
		t.Fatalf("NewDevice: %v, want a disabled mesh rather than an error", err)
	}
	d, ok := m.(*Disabled)
	if !ok {
		t.Fatalf("got %T, want *Disabled", m)
	}
	if !strings.Contains(d.Reason(), "privileges") {
		t.Errorf("Reason = %q, want it to name the privilege problem", d.Reason())
	}

	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Backend != BackendDisabled {
		t.Errorf("Backend = %q, want %q", st.Backend, BackendDisabled)
	}
}

func TestNewDevice_UsesTheRealInterfaceName(t *testing.T) {
	// Darwin's utun numbering ignores the requested name entirely, so
	// every later log line and link call must use what came back.
	tunDev := &fakeTUN{name: "utun7"}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(tunDev, &fakeWG{}, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() {
		if cerr := m.Close(); cerr != nil {
			t.Errorf("Close: %v", cerr)
		}
	}()

	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Interface != "utun7" {
		t.Errorf("Interface = %q, want the name the kernel actually gave us", st.Interface)
	}
}

func TestNewDevice_ClosesTUNWhenBringUpFails(t *testing.T) {
	tunDev := &fakeTUN{name: "acme0"}
	_, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(tunDev, &fakeWG{upErr: errors.New("boom")}, BackendUserspace))
	if err == nil {
		t.Fatal("want an error when the device fails to come up")
	}
	if !tunDev.closed {
		t.Error("the TUN device was leaked after a failed startup")
	}
}

func TestDevice_ApplyReadsCurrentPeersBeforeWriting(t *testing.T) {
	stale := testKey(9)
	wanted := testKey(1)

	wg := &fakeWG{getResponse: "public_key=" + stale.Hex() + "\nerrno=0\n\n"}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, wg, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	cfg := DeviceConfig{
		NodeID:     "self",
		PrivateKey: testKey(100),
		Address:    netip.MustParsePrefix("10.181.0.1/16"),
		ListenPort: 51820,
		Peers: []PeerConfig{{
			NodeID:     "a",
			PublicKey:  wanted,
			AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.181.0.2/32")},
		}},
	}
	if err := m.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := wg.lastSet(t)
	if !strings.Contains(got, "public_key="+stale.Hex()+"\nremove=true") {
		t.Errorf("the peer the device already had, which the plan no longer names, was not removed\ngot:\n%s", got)
	}
	if !strings.Contains(got, "public_key="+wanted.Hex()) {
		t.Errorf("the planned peer was not written\ngot:\n%s", got)
	}
}

func TestDevice_ApplyFailures(t *testing.T) {
	tests := []struct {
		name string
		wg   *fakeWG
		want string
	}{
		{
			name: "cannot read the current state",
			wg:   &fakeWG{getErr: errors.New("ipc closed")},
			want: "read current state",
		},
		{
			name: "current state does not parse",
			wg:   &fakeWG{getResponse: "garbage without an equals sign\n"},
			want: "parse current state",
		},
		{
			name: "device rejects the config",
			wg:   &fakeWG{setErr: errors.New("invalid key")},
			want: "configure device",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewDevice(context.Background(), privilegedProbe(),
				WithLogger(quietLogger()), WithShortName("acme"),
				withFakes(&fakeTUN{name: "acme0"}, tc.wg, BackendUserspace))
			if err != nil {
				t.Fatalf("NewDevice: %v", err)
			}
			defer func() { _ = m.Close() }()

			err = m.Apply(context.Background(), DeviceConfig{NodeID: "self"})
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
			// Every error must name the node it was for, matching this
			// codebase's structured logging convention of resource IDs
			// on every log line.
			if !strings.Contains(err.Error(), `"self"`) {
				t.Errorf("error = %q, want it to name the node ID", err)
			}
		})
	}
}

// fakeLink records what the link configurator was asked to do.
type fakeLink struct {
	calls []netip.Prefix
	iface string
	err   error
}

func (f *fakeLink) SetAddress(_ context.Context, iface string, addr netip.Prefix) error {
	f.iface = iface
	f.calls = append(f.calls, addr)
	return f.err
}

func TestDevice_ConfiguresTheLinkWhenGivenOne(t *testing.T) {
	link := &fakeLink{}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"), WithLinkConfigurator(link),
		withFakes(&fakeTUN{name: "acme0"}, &fakeWG{}, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	cfg := DeviceConfig{NodeID: "self", Address: netip.MustParsePrefix("10.181.0.1/16")}
	if err := m.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(link.calls) != 1 || link.calls[0] != cfg.Address {
		t.Fatalf("SetAddress calls = %v, want [%s]", link.calls, cfg.Address)
	}
	if link.iface != "acme0" {
		t.Errorf("SetAddress got interface %q, want acme0", link.iface)
	}

	// Level-triggered: applying the same config again calls SetAddress
	// again, which is why implementations must be idempotent.
	if err := m.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(link.calls) != 2 {
		t.Errorf("SetAddress calls = %d, want 2", len(link.calls))
	}
}

func TestDevice_LinkFailureIsAnApplyFailure(t *testing.T) {
	link := &fakeLink{err: errors.New("permission denied")}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"), WithLinkConfigurator(link),
		withFakes(&fakeTUN{name: "acme0"}, &fakeWG{}, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	err = m.Apply(context.Background(), DeviceConfig{
		NodeID: "self", Address: netip.MustParsePrefix("10.181.0.1/16"),
	})
	if err == nil {
		t.Fatal("want an error when the interface cannot be addressed")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want the underlying cause preserved", err)
	}
}

func TestDevice_NoLinkConfiguratorStillApplies(t *testing.T) {
	// Without a link configurator the WireGuard device is still
	// configured; it is only the interface addressing that is missing,
	// and the device warns about that rather than failing.
	wg := &fakeWG{}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, wg, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	err = m.Apply(context.Background(), DeviceConfig{
		NodeID: "self", Address: netip.MustParsePrefix("10.181.0.1/16"), ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(wg.lastSet(t), "listen_port=51820") {
		t.Error("the WireGuard device was not configured")
	}
}

func TestDevice_StatusNamesPeersByNodeID(t *testing.T) {
	peer := testKey(1)
	wg := &fakeWG{}
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, wg, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	cfg := DeviceConfig{
		NodeID:  "self",
		Address: netip.MustParsePrefix("10.181.0.1/16"),
		Peers:   []PeerConfig{{NodeID: "node-a", PublicKey: peer}},
	}
	if err := m.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	wg.mu.Lock()
	wg.getResponse = "public_key=" + peer.Hex() + "\nerrno=0\n\n"
	wg.mu.Unlock()

	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Backend != BackendUserspace {
		t.Errorf("Backend = %q, want %q", st.Backend, BackendUserspace)
	}
	if st.Address != cfg.Address {
		t.Errorf("Address = %s, want %s", st.Address, cfg.Address)
	}
	if len(st.Peers) != 1 || st.Peers[0].NodeID != "node-a" {
		t.Fatalf("peers = %+v, want one named node-a", st.Peers)
	}
}

// TestDevice_ReportsTheDetectedBackend pins the honest half of the
// kernel gap NewDevice documents: a node where Detect says "kernel" still
// runs wireguard-go today, and Status has to say "kernel" because that is
// what was detected, so an operator reading it can tell that the fast
// path was available. When the kernel backend is actually implemented,
// this test is what stops the two from silently diverging.
func TestDevice_ReportsTheDetectedBackend(t *testing.T) {
	m, err := NewDevice(context.Background(), fakeProbe{os: "linux", moduleLoad: true, privileged: true},
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, &fakeWG{}, BackendKernel))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Backend != BackendKernel {
		t.Errorf("Backend = %q, want %q", st.Backend, BackendKernel)
	}
}

func TestDevice_ClosedIsAnError(t *testing.T) {
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, &fakeWG{}, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close must be a no-op, got %v", err)
	}
	if err := m.Apply(context.Background(), DeviceConfig{NodeID: "self"}); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("Apply after Close = %v, want it to wrap %v", err, ErrMeshClosed)
	}
	if _, err := m.Status(context.Background()); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("Status after Close = %v, want it to wrap %v", err, ErrMeshClosed)
	}
}

func TestDevice_RespectsContextCancellation(t *testing.T) {
	m, err := NewDevice(context.Background(), privilegedProbe(),
		WithLogger(quietLogger()), WithShortName("acme"),
		withFakes(&fakeTUN{name: "acme0"}, &fakeWG{}, BackendUserspace))
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := m.Apply(ctx, DeviceConfig{NodeID: "self"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Apply = %v, want it to wrap context.Canceled", err)
	}
	if _, err := m.Status(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Status = %v, want it to wrap context.Canceled", err)
	}
}

func TestDisabled_RecordsIntentWithoutPretendingToWork(t *testing.T) {
	d := NewDisabled("single node deployment")
	cfg := DeviceConfig{
		NodeID:     "self",
		Address:    netip.MustParsePrefix("10.181.0.1/16"),
		ListenPort: 51820,
		Peers:      []PeerConfig{{NodeID: "a", PublicKey: testKey(1), Endpoint: "203.0.113.2:51820"}},
	}
	if err := d.Apply(context.Background(), cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	st, err := d.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Backend != BackendDisabled {
		t.Errorf("Backend = %q, want %q", st.Backend, BackendDisabled)
	}
	if len(st.Peers) != 1 {
		t.Fatalf("got %d peers, want the configured one to be visible", len(st.Peers))
	}
	// A configured-but-not-actually-connected peer must read as
	// unreachable, not as healthy-with-no-data.
	if st.Peers[0].Healthy(time.Now(), staleAfter) {
		t.Error("a peer on a disabled mesh reports as healthy")
	}
	if d.LocalAddress() != cfg.Address {
		t.Errorf("LocalAddress = %s, want %s", d.LocalAddress(), cfg.Address)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := d.Apply(context.Background(), cfg); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("Apply after Close = %v, want it to wrap %v", err, ErrMeshClosed)
	}
	if _, err := d.Status(context.Background()); !errors.Is(err, ErrMeshClosed) {
		t.Errorf("Status after Close = %v, want it to wrap %v", err, ErrMeshClosed)
	}
}
