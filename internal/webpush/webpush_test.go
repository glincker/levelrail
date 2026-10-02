package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

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

	if err := sender.Send(context.Background(), "title", "body"); err == nil {
		t.Error("Send() error = nil, want an error when no browser has registered")
	}
}

func TestSender_Send_NoVAPIDKeys_Errors(t *testing.T) {
	subs := &fakeSubscriptions{}
	sender := NewSender(subs, "", "", "ops@example.com", nil, nil)

	if err := sender.Send(context.Background(), "title", "body"); err == nil {
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
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", srv.Client(), nil)

	if err := sender.Send(context.Background(), "Deploy succeeded", "web -> web:1"); err != nil {
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
	sender := NewSender(subs, publicKey, privateKey, "ops@example.com", srv.Client(), nil)

	err = sender.Send(context.Background(), "title", "body")
	if err == nil {
		t.Fatal("Send() error = nil, want an error when every subscription is gone")
	}
	if len(subs.deleted) != 1 || subs.deleted[0] != "push_1" {
		t.Errorf("deleted = %v, want [\"push_1\"] pruned after a 410 response", subs.deleted)
	}
}
