package webpush

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/store"
)

type allowAll struct{}

func (allowAll) AllowPush(context.Context, string, alerting.PushAudience) bool { return true }

// userPolicy allows only the listed users.
type userPolicy map[string]bool

func (p userPolicy) AllowPush(_ context.Context, id string, _ alerting.PushAudience) bool {
	return p[id]
}

// fakeKeyStore is an in-memory internal/secrets.Manager stand-in: enough
// of KeyStore's surface for EnsureVAPIDKeys, no envelope encryption
// involved since that package has its own dedicated tests.
type fakeKeyStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeKeyStore() *fakeKeyStore { return &fakeKeyStore{values: map[string]string{}} }

func (f *fakeKeyStore) key(serviceName, envKey string) string { return serviceName + "/" + envKey }

func (f *fakeKeyStore) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[f.key(serviceName, envKey)]
	return ok, nil
}

func (f *fakeKeyStore) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[f.key(serviceName, envKey)] = plaintext
	return nil
}

func (f *fakeKeyStore) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[f.key(serviceName, envKey)]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func TestEnsureVAPIDKeys_GeneratesOnce(t *testing.T) {
	keys := newFakeKeyStore()

	public1, private1, err := EnsureVAPIDKeys(context.Background(), keys)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() error = %v", err)
	}
	if public1 == "" || private1 == "" {
		t.Fatal("EnsureVAPIDKeys() returned an empty key")
	}

	public2, private2, err := EnsureVAPIDKeys(context.Background(), keys)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() second call error = %v", err)
	}
	if public1 != public2 || private1 != private2 {
		t.Error("EnsureVAPIDKeys() regenerated a keypair on a second call, want the first one reused")
	}
}

// fakeSubscriptions is an in-memory Subscriptions stand-in.
type fakeSubscriptions struct {
	mu      sync.Mutex
	subs    []store.PushSubscription
	deleted []string
}

func (f *fakeSubscriptions) ListPushSubscriptions(_ context.Context) ([]store.PushSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.PushSubscription(nil), f.subs...), nil
}

func (f *fakeSubscriptions) DeletePushSubscriptionByID(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	return nil
}

// newFakeBrowserKeys mints a syntactically valid P256dh/Auth pair the
// same shape a real browser's PushSubscription.getKey() would hand back:
// webpush-go's own encryption math needs a real point on the P256 curve,
// not an arbitrary string.
func newFakeBrowserKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate browser key: %v", err)
	}
	p256dh = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())

	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("generate auth secret: %v", err)
	}
	auth = base64.RawURLEncoding.EncodeToString(authBytes)
	return p256dh, auth
}

func TestSender_Send_NoSubscriptions_Errors(t *testing.T) {
	keys := newFakeKeyStore()
	publicKey, privateKey, err := EnsureVAPIDKeys(context.Background(), keys)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() error = %v", err)
	}
	subs := &fakeSubscriptions{}
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", nil, nil)

	if err := sender.Send(context.Background(), "title", "body", alerting.PushAudience{}); err == nil {
		t.Error("Send() error = nil, want an error when no browser has registered")
	}
}

func TestSender_Send_NoVAPIDKeys_Errors(t *testing.T) {
	subs := &fakeSubscriptions{}
	sender := NewSender(subs, "", "", "ops@example.com", nil, nil)

	if err := sender.Send(context.Background(), "title", "body", alerting.PushAudience{}); err == nil {
		t.Error("Send() error = nil, want an error when vapid keys are not configured")
	}
}

func TestSender_Send_DeliversToEveryRegisteredSubscription(t *testing.T) {
	var received int32
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		received++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	keys := newFakeKeyStore()
	publicKey, privateKey, err := EnsureVAPIDKeys(context.Background(), keys)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() error = %v", err)
	}

	p256dh1, auth1 := newFakeBrowserKeys(t)
	p256dh2, auth2 := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{
		{ID: "push_1", Endpoint: srv.URL, P256dh: p256dh1, Auth: auth1},
		{ID: "push_2", Endpoint: srv.URL, P256dh: p256dh2, Auth: auth2},
	}}
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", srv.Client(), nil).WithPolicy(allowAll{}).WithEndpointValidator(func(string) error { return nil })

	if err := sender.Send(context.Background(), "Deploy succeeded", "web -> web:1", alerting.PushAudience{}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	got := received
	mu.Unlock()
	if got != 2 {
		t.Errorf("receiver got %d requests, want 2 (one per subscription)", got)
	}
}

func TestSender_Send_PrunesExpiredSubscription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer srv.Close()

	keys := newFakeKeyStore()
	publicKey, privateKey, err := EnsureVAPIDKeys(context.Background(), keys)
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() error = %v", err)
	}

	p256dh, auth := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{
		{ID: "push_1", Endpoint: srv.URL, P256dh: p256dh, Auth: auth},
	}}
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", srv.Client(), nil).WithPolicy(allowAll{}).WithEndpointValidator(func(string) error { return nil })

	err = sender.Send(context.Background(), "title", "body", alerting.PushAudience{})
	if err == nil {
		t.Fatal("Send() error = nil, want an error when every subscription is gone")
	}
	if len(subs.deleted) != 1 || subs.deleted[0] != "push_1" {
		t.Errorf("deleted = %v, want [\"push_1\"] pruned after a 410 response", subs.deleted)
	}
}

func newTestSender(t *testing.T, subs Subscriptions, client *http.Client, p RecipientPolicy) *Sender {
	t.Helper()
	publicKey, privateKey, err := EnsureVAPIDKeys(context.Background(), newFakeKeyStore())
	if err != nil {
		t.Fatalf("EnsureVAPIDKeys() error = %v", err)
	}
	return NewSender(subs, publicKey, privateKey, "ops@example.com", client, nil).WithPolicy(p).WithEndpointValidator(func(string) error { return nil })
}

func TestSender_Send_OnlyAllowedUsersReceive(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pa, aa := newFakeBrowserKeys(t)
	pb, ab := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{
		{ID: "push_root", UserID: "root", Endpoint: srv.URL + "/root", P256dh: pa, Auth: aa},
		{ID: "push_viewer", UserID: "viewer", Endpoint: srv.URL + "/viewer", P256dh: pb, Auth: ab},
	}}
	sender := newTestSender(t, subs, srv.Client(), userPolicy{"root": true})

	if err := sender.Send(context.Background(), "t", "b", alerting.PushAudience{}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["/root"] != 1 || hits["/viewer"] != 0 {
		t.Errorf("hits = %v, want only root to receive", hits)
	}
}

func TestSender_Send_NoPolicyOrNobodyAllowed_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) }))
	defer srv.Close()
	p, a := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{{ID: "push_1", UserID: "u", Endpoint: srv.URL, P256dh: p, Auth: a}}}

	for name, policy := range map[string]RecipientPolicy{"nil": nil, "none": userPolicy{}} {
		if err := newTestSender(t, subs, srv.Client(), policy).Send(context.Background(), "t", "b", alerting.PushAudience{}); err == nil {
			t.Errorf("%s policy: Send() error = nil, want an error", name)
		}
	}
}

func TestSender_Send_HangingEndpointStopsAtTimeout(t *testing.T) {
	t.Setenv(EnvSendTimeout, "300ms")
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	p, a := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{{ID: "push_1", UserID: "u", Endpoint: srv.URL, P256dh: p, Auth: a}}}
	sender := newTestSender(t, subs, srv.Client(), allowAll{})

	start := time.Now()
	err := sender.Send(context.Background(), "t", "b", alerting.PushAudience{})
	if err == nil {
		t.Fatal("Send() error = nil, want a timeout failure")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Send() took %v, want it to stop near the 300ms timeout", elapsed)
	}
}

func TestSender_Send_RedirectNotFollowed(t *testing.T) {
	var followed int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&followed, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	p, a := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{{ID: "push_1", UserID: "u", Endpoint: srv.URL, P256dh: p, Auth: a}}}
	if err := newTestSender(t, subs, srv.Client(), allowAll{}).Send(context.Background(), "t", "b", alerting.PushAudience{}); err == nil {
		t.Error("Send() error = nil, want a failure for a redirect response")
	}
	if atomic.LoadInt32(&followed) != 0 {
		t.Error("redirect target was contacted, want redirects refused")
	}
}

func TestSender_Send_ErrorsNeverContainEndpoint(t *testing.T) {
	// A closed port makes the transport fail with a url.Error naming the endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL + "/secret-token-path"
	srv.Close()

	p, a := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{{ID: "push_1", UserID: "u", Endpoint: endpoint, P256dh: p, Auth: a}}}
	var logs bytes.Buffer
	sender := newTestSender(t, subs, &http.Client{}, allowAll{})
	sender.logger = slog.New(slog.NewTextHandler(&logs, nil))

	err := sender.Send(context.Background(), "t", "b", alerting.PushAudience{})
	if err == nil {
		t.Fatal("Send() error = nil, want a failure")
	}
	for _, text := range []string{err.Error(), logs.String()} {
		if strings.Contains(text, "secret-token-path") || strings.Contains(text, endpoint) {
			t.Errorf("output contains the endpoint: %q", text)
		}
	}
}

func TestEncodePayload_TruncatesLargeBody(t *testing.T) {
	out, err := encodePayload("title", strings.Repeat("\u00e9\"x", 10000))
	if err != nil {
		t.Fatalf("encodePayload() error = %v", err)
	}
	if len(out) > maxPayloadBytes {
		t.Errorf("payload = %d bytes, want at most %d", len(out), maxPayloadBytes)
	}
	var got pushPayload
	if err := json.Unmarshal(out, &got); err != nil || got.Body == "" || !utf8.ValidString(got.Body) {
		t.Errorf("payload not valid truncated JSON: err=%v body=%q", err, got.Body)
	}
}

func TestValidateEndpoint(t *testing.T) {
	allowed := AllowedHosts(func(string) string { return "" })
	for endpoint, ok := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":                 true,
		"https://updates.push.services.mozilla.com/wpush/v2/abc":  true,
		"https://wns2-par02p.notify.windows.com/w/?token=abc":     true,
		"https://web.push.apple.com/abc":                          true,
		"http://fcm.googleapis.com/fcm/send/abc":                  false,
		"https://evil.example.com/fcm.googleapis.com":             false,
		"https://fcm.googleapis.com.evil.example.com/x":           false,
		"https://user@fcm.googleapis.com/x":                       false,
		"https://fcm.googleapis.com:8443/x":                       false,
		"https://127.0.0.1/x":                                     false,
		"":                                                        false,
		"https://fcm.googleapis.com/" + strings.Repeat("a", 3000): false,
	} {
		if got := ValidateEndpoint(endpoint, allowed) == nil; got != ok {
			t.Errorf("ValidateEndpoint(%q) accepted = %v, want %v", endpoint, got, ok)
		}
	}
	if ValidateEndpoint("https://push.example.org/x", AllowedHosts(func(string) string { return "push.example.org" })) != nil {
		t.Error("env allow list host was refused")
	}
}

func TestValidateKeys(t *testing.T) {
	p, a := newFakeBrowserKeys(t)
	if err := ValidateKeys(p, a); err != nil {
		t.Fatalf("ValidateKeys(valid) = %v", err)
	}
	short := base64.RawURLEncoding.EncodeToString(make([]byte, 8))
	for name, pair := range map[string][2]string{
		"short auth":     {p, short},
		"not a point":    {base64.RawURLEncoding.EncodeToString(make([]byte, 65)), a},
		"short p256dh":   {short, a},
		"not base64":     {"!!!", a},
		"oversized auth": {p, strings.Repeat("A", 200)},
	} {
		if ValidateKeys(pair[0], pair[1]) == nil {
			t.Errorf("%s: ValidateKeys() = nil, want an error", name)
		}
	}
}

func TestSender_Send_PrunesDisallowedStoredEndpoint(t *testing.T) {
	var hit int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hit, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	p, a := newFakeBrowserKeys(t)
	subs := &fakeSubscriptions{subs: []store.PushSubscription{{ID: "push_old", UserID: "u", Endpoint: srv.URL, P256dh: p, Auth: a}}}
	publicKey, privateKey, _ := EnsureVAPIDKeys(context.Background(), newFakeKeyStore())
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", srv.Client(), nil).WithPolicy(allowAll{})

	if err := sender.Send(context.Background(), "t", "b", alerting.PushAudience{}); err == nil {
		t.Error("Send() error = nil, want a failure for a disallowed stored endpoint")
	}
	if atomic.LoadInt32(&hit) != 0 {
		t.Error("a disallowed endpoint was contacted")
	}
	if len(subs.deleted) != 1 || subs.deleted[0] != "push_old" {
		t.Errorf("deleted = %v, want the row pruned", subs.deleted)
	}
}
