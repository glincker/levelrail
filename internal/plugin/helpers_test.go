package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const testDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

func manifestYAML(abilities string) []byte {
	const name = "example"
	return []byte("api_version: 1\nname: " + name + "\nversion: 1.0.0\nkind: notification_channel\npublisher: Example Org\n" +
		"image: registry.example.com/team/" + name + "@" + testDigest + "\nabilities: " + abilities + "\noutbound_hosts: []\n")
}

type memStore struct {
	mu      sync.Mutex
	plugins map[string]Record
	keys    []TrustedKey
}

func newMemStore() *memStore { return &memStore{plugins: map[string]Record{}} }

func (s *memStore) ListPlugins(context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Record{}
	for _, r := range s.plugins {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *memStore) GetPlugin(_ context.Context, name string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.plugins[name]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}

func (s *memStore) InsertPlugin(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plugins[r.Name]; ok {
		return ErrExists
	}
	s.plugins[r.Name] = r
	return nil
}

func (s *memStore) UpdatePlugin(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.plugins[r.Name]
	if !ok {
		return ErrNotFound
	}
	r.Kind = cur.Kind
	s.plugins[r.Name] = r
	return nil
}

func (s *memStore) SetLastError(_ context.Context, name, msg, at string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.plugins[name]; ok {
		r.LastError, r.UpdatedAt = msg, at
		s.plugins[name] = r
	}
	return nil
}

func (s *memStore) DeletePlugin(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plugins, name)
	return nil
}

func (s *memStore) ListKeys(context.Context) ([]TrustedKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TrustedKey{}, s.keys...), nil
}

func (s *memStore) AddKey(_ context.Context, k TrustedKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, k)
	return nil
}

func (s *memStore) RemoveKey(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, k := range s.keys {
		if k.KeyID == id {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

type memAudit struct {
	mu     sync.Mutex
	events []AuditEvent
	fail   error
}

func (a *memAudit) Record(_ context.Context, e AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fail != nil {
		return a.fail
	}
	a.events = append(a.events, e)
	return nil
}

func (a *memAudit) actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for _, e := range a.events {
		out = append(out, e.Action)
	}
	return out
}

type fakeDocker struct {
	mu         sync.Mutex
	containers map[string]*docker.ContainerState
	specs      []docker.ContainerSpec
	nextID     int
	networks   []string

	createErr error
	startErr  error
	removeErr error
	removed   []string
}

func newFakeDocker() *fakeDocker { return &fakeDocker{containers: map[string]*docker.ContainerState{}} }

func (f *fakeDocker) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.containers[name]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeDocker) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []docker.ContainerState{}
	for n, c := range f.containers {
		if strings.HasPrefix(n, prefix) {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeDocker) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return "", f.createErr
	}
	f.nextID++
	id := fmt.Sprintf("c%d", f.nextID)
	f.containers[spec.Name] = &docker.ContainerState{ID: id, Name: spec.Name, Labels: spec.Labels}
	f.specs = append(f.specs, spec)
	return id, nil
}

func (f *fakeDocker) Start(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return f.startErr
	}
	for _, c := range f.containers {
		if c.ID == id {
			c.Running = true
		}
	}
	return nil
}

func (f *fakeDocker) Stop(_ context.Context, id string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.containers {
		if c.ID == id {
			c.Running = false
		}
	}
	return nil
}

func (f *fakeDocker) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return f.removeErr
	}
	for n, c := range f.containers {
		if c.ID == id {
			delete(f.containers, n)
			f.removed = append(f.removed, id)
		}
	}
	return nil
}

func (f *fakeDocker) EnsureNetwork(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.networks = append(f.networks, name)
	return name, nil
}

func (f *fakeDocker) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.containers)
}

type fakeRPC struct {
	mu          sync.Mutex
	describe    *pluginpb.DescribeResponse
	describeErr error
	healthOK    bool
	sendResp    *pluginpb.SendResponse
	sendErr     error
	sent        []*pluginpb.SendRequest
	block       chan struct{}
	panelResp   *pluginpb.PanelDataResponse
	panelErr    error
	panelReqs   []*pluginpb.PanelDataRequest
	actionResp  *pluginpb.PanelActionResponse
	actionReqs  []*pluginpb.PanelActionRequest
}

func (r *fakeRPC) PanelData(ctx context.Context, req *pluginpb.PanelDataRequest) (*pluginpb.PanelDataResponse, error) {
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.panelReqs = append(r.panelReqs, req)
	return r.panelResp, r.panelErr
}

func (r *fakeRPC) PanelAction(_ context.Context, req *pluginpb.PanelActionRequest) (*pluginpb.PanelActionResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actionReqs = append(r.actionReqs, req)
	return r.actionResp, r.panelErr
}

func (r *fakeRPC) Describe(context.Context, *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	return r.describe, r.describeErr
}

func (r *fakeRPC) Health(context.Context, *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &pluginpb.HealthResponse{Ok: r.healthOK}, nil
}

func (r *fakeRPC) Send(ctx context.Context, req *pluginpb.SendRequest) (*pluginpb.SendResponse, error) {
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, req)
	if r.sendErr != nil {
		return nil, r.sendErr
	}
	return r.sendResp, nil
}

type closeFn func() error

func (c closeFn) Close() error { return c() }

type harness struct {
	t     *testing.T
	m     *Manager
	store *memStore
	dk    *fakeDocker
	audit *memAudit
	rpc   *fakeRPC
	now   time.Time
	priv  ed25519.PrivateKey
	keyID string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t: t, store: newMemStore(), dk: newFakeDocker(), audit: &memAudit{}, priv: priv, keyID: Fingerprint(pub),
		now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC),
		rpc: &fakeRPC{healthOK: true, sendResp: &pluginpb.SendResponse{Delivered: true, Detail: "ok"}},
	}
	h.m = NewManager(h.store, h.dk, h.audit, Config{Namespace: "ns", DataDir: t.TempDir(), UID: 1000, GID: 1000, MaxConcurrent: 2, BreakerFailures: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.m.now = func() time.Time { return h.now }
	h.m.dial = func(context.Context, string, int, int) (rpcClient, io.Closer, error) {
		return h.rpc, closeFn(func() error { return nil }), nil
	}
	h.store.keys = []TrustedKey{{KeyID: h.keyID, Name: "pub", PublicKey: b64(pub)}}
	h.setDescribe("example", []string{AbilityRead})
	return h
}

func (h *harness) setDescribe(name string, abilities []string) {
	h.rpc.describe = &pluginpb.DescribeResponse{ContractVersion: 1, Name: name, Version: "1.0.0", Kind: KindNotificationChannel, RequestedAbilities: abilities}
}

func (h *harness) request(manifest []byte) InstallRequest {
	return InstallRequest{Manifest: manifest, KeyID: h.keyID, Signature: Sign(h.priv, manifest, testDigest)}
}

func (h *harness) installApprovedEnabled(abilities string, approve []string) {
	h.t.Helper()
	ctx := context.Background()
	if _, err := h.m.Install(ctx, h.request(manifestYAML(abilities)), "admin"); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.m.Approve(ctx, "example", approve, "admin"); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

var errBoom = errors.New("boom")
