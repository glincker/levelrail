package alerting

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	testSlackToken  = "T000/B000/XXXXsupersecretXXXX" //nolint:gosec // test fixture, not a credential
	testInteractive = "0123456789abcdefsigningsecret"
	testSlackURL    = "https://hooks.slack.com/services/" + testSlackToken
)

func newChannelSecretsManager(t *testing.T) (*secrets.Manager, *store.DB) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	return secrets.NewManager(st, mk), st
}

func rawChannelColumns(t *testing.T, db *DB, id string) (notifyURL, interactive string) {
	t.Helper()
	if err := db.QueryRowContext(context.Background(),
		`SELECT notify_url, interactive_secret FROM notification_channels WHERE id = ?`, id).Scan(&notifyURL, &interactive); err != nil {
		t.Fatalf("raw select %q: %v", id, err)
	}
	return notifyURL, interactive
}

func assertNoPlaintext(t *testing.T, db *DB, id string, plaintexts ...string) {
	t.Helper()
	u, s := rawChannelColumns(t, db, id)
	for _, p := range plaintexts {
		if strings.Contains(u, p) || strings.Contains(s, p) {
			t.Fatalf("row %q still carries plaintext %q: notify_url=%q interactive_secret=%q", id, p, u, s)
		}
	}
}

func slackChannel(id string) NotificationChannel {
	return NotificationChannel{
		ID: id, Name: "Team Slack", Kind: NotifySlack, NotifyURL: testSlackURL, Enabled: true,
		InteractiveApprovals: true, InteractiveSecret: testInteractive,
	}
}

func TestSaveNotificationChannel_SealsSecretsUnderEnvelope(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	mgr, _ := newChannelSecretsManager(t)
	db.SetChannelSecrets(mgr)
	ctx := context.Background()

	url := "https://hooks.slack.com/services/" + testSlackToken
	if err := db.SaveNotificationChannel(ctx, slackChannel("chn_1")); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}
	assertNoPlaintext(t, db, "chn_1", testSlackToken, testInteractive)

	got, err := db.GetNotificationChannel(ctx, "chn_1")
	if err != nil {
		t.Fatalf("GetNotificationChannel() error = %v", err)
	}
	if got.NotifyURL != "https://hooks.slack.com/(hidden)" {
		t.Errorf("listed NotifyURL = %q, want the masked form", got.NotifyURL)
	}
	if got.InteractiveSecret != MaskedChannelSecret {
		t.Errorf("listed InteractiveSecret = %q, want %q", got.InteractiveSecret, MaskedChannelSecret)
	}

	revealed, err := db.RevealNotificationChannel(ctx, *got)
	if err != nil {
		t.Fatalf("RevealNotificationChannel() error = %v", err)
	}
	if revealed.NotifyURL != url || revealed.InteractiveSecret != testInteractive {
		t.Errorf("revealed = %q/%q, want the saved plaintext", revealed.NotifyURL, revealed.InteractiveSecret)
	}

	// Saving the masked values back (an edit that changed only the name) keeps the sealed secrets.
	got.Name = "Renamed"
	if err := db.SaveNotificationChannel(ctx, *got); err != nil {
		t.Fatalf("re-save masked: %v", err)
	}
	again, _ := db.GetNotificationChannel(ctx, "chn_1")
	revealed, err = db.RevealNotificationChannel(ctx, *again)
	if err != nil || revealed.NotifyURL != url || revealed.InteractiveSecret != testInteractive {
		t.Fatalf("after masked re-save revealed = %+v, err = %v", revealed, err)
	}

	if err := db.DeleteNotificationChannel(ctx, "chn_1"); err != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", err)
	}
	if _, err := mgr.Resolve(ctx, NotificationChannelSecretsKey("chn_1"), channelSecretNotifyURL); !errors.Is(err, secrets.ErrValueNotFound) {
		t.Errorf("secret after delete: err = %v, want ErrValueNotFound", err)
	}
}

func TestSaveNotificationChannel_EmailAddressStaysReadable(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	mgr, _ := newChannelSecretsManager(t)
	db.SetChannelSecrets(mgr)
	ctx := context.Background()
	c := NotificationChannel{ID: "chn_mail", Name: "Ops", Kind: NotifyEmail, NotifyURL: "ops@example.com", Enabled: true}
	if err := db.SaveNotificationChannel(ctx, c); err != nil {
		t.Fatal(err)
	}
	if u, _ := rawChannelColumns(t, db, "chn_mail"); u != "ops@example.com" {
		t.Errorf("email notify_url column = %q, want the address unchanged", u)
	}
}

func TestSaveNotificationChannel_NoManagerKeepsWorking(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	url := "https://hooks.slack.com/services/" + testSlackToken
	if err := db.SaveNotificationChannel(ctx, slackChannel("chn_1")); err != nil {
		t.Fatalf("SaveNotificationChannel() error = %v", err)
	}
	got, _ := db.GetNotificationChannel(ctx, "chn_1")
	revealed, err := db.RevealNotificationChannel(ctx, *got)
	if err != nil || revealed.NotifyURL != url {
		t.Fatalf("reveal without a manager = %q, %v", revealed.NotifyURL, err)
	}
	n, err := db.CountPlaintextNotificationChannels(ctx)
	if err != nil || n != 1 {
		t.Fatalf("CountPlaintextNotificationChannels() = %d, %v, want 1", n, err)
	}
	if _, err := db.SealNotificationChannelSecrets(ctx); !errors.Is(err, ErrChannelSecretsUnavailable) {
		t.Fatalf("seal without a manager err = %v, want ErrChannelSecretsUnavailable", err)
	}
}

func TestSealNotificationChannelSecrets_MigratesPlaintextRowsIdempotently(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	slackURL := "https://hooks.slack.com/services/" + testSlackToken
	tgURL := "https://api.telegram.org/botSECRETBOT:TOKEN/sendMessage?chat_id=42"
	for _, c := range []NotificationChannel{
		slackChannel("chn_slack"),
		{ID: "chn_tg", Name: "TG", Kind: NotifyTelegram, NotifyURL: tgURL, Enabled: true},
		{ID: "chn_pd", Name: "PD", Kind: NotifyPagerDuty, NotifyURL: "pd-routing-key-123", Enabled: true},
		{ID: "chn_mail", Name: "Mail", Kind: NotifyEmail, NotifyURL: "ops@example.com", Enabled: true},
		{ID: "chn_push", Name: "Push", Kind: NotifyWebpush, Enabled: true},
	} {
		if err := db.SaveNotificationChannel(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	mgr, st := newChannelSecretsManager(t)
	db.SetChannelSecrets(mgr)
	sealed, err := db.SealNotificationChannelSecrets(ctx)
	if err != nil {
		t.Fatalf("SealNotificationChannelSecrets() error = %v", err)
	}
	if sealed != 3 {
		t.Errorf("sealed = %d, want 3 (slack, telegram, pagerduty)", sealed)
	}
	assertNoPlaintext(t, db, "chn_slack", testSlackToken, testInteractive)
	assertNoPlaintext(t, db, "chn_tg", "SECRETBOT")
	assertNoPlaintext(t, db, "chn_pd", "pd-routing-key-123")
	if n, _ := db.CountPlaintextNotificationChannels(ctx); n != 0 {
		t.Errorf("plaintext rows after seal = %d, want 0", n)
	}

	total, withPrefix, err := st.CountSecretValuesByPrefix(ctx, []byte{0xB7, 0x1C, 0xE5, 0x5A, 0x01})
	if err != nil || total != 4 || withPrefix != 4 {
		t.Errorf("stored ciphertexts total=%d bound=%d err=%v, want 4 slot-bound values", total, withPrefix, err)
	}

	for id, want := range map[string]string{"chn_slack": slackURL, "chn_tg": tgURL, "chn_pd": "pd-routing-key-123"} {
		c, _ := db.GetNotificationChannel(ctx, id)
		r, err := db.RevealNotificationChannel(ctx, *c)
		if err != nil || r.NotifyURL != want {
			t.Errorf("%s revealed %q, %v; want %q", id, r.NotifyURL, err, want)
		}
	}

	again, err := db.SealNotificationChannelSecrets(ctx)
	if err != nil || again != 0 {
		t.Errorf("second run sealed %d, err %v; want 0, nil", again, err)
	}
}

// lyingSecrets stores values but returns something else on read, so the
// migration's verify step must refuse to clear the plaintext column.
type lyingSecrets struct {
	mu     sync.Mutex
	stored map[string]string
}

func (l *lyingSecrets) SetValue(_ context.Context, svc, key, v string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stored == nil {
		l.stored = map[string]string{}
	}
	l.stored[svc+"/"+key] = v
	return nil
}

func (l *lyingSecrets) Resolve(context.Context, string, string) (string, error) {
	return "not-what-was-stored", nil
}

func (l *lyingSecrets) DeleteAll(context.Context, string) error { return nil }

func TestSealNotificationChannelSecrets_VerifyFailureKeepsPlaintext(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	url := "https://hooks.slack.com/services/" + testSlackToken
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "S", Kind: NotifySlack, NotifyURL: url, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	db.SetChannelSecrets(&lyingSecrets{})
	if _, err := db.SealNotificationChannelSecrets(ctx); err == nil {
		t.Fatal("seal with a failed round-trip returned nil error")
	}
	if u, _ := rawChannelColumns(t, db, "chn_1"); u != url {
		t.Fatalf("notify_url column = %q, want the plaintext kept after a failed verify", u)
	}
}

func TestSealNotificationChannelSecrets_ResumesAfterInterruption(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	url := "https://hooks.slack.com/services/" + testSlackToken
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "S", Kind: NotifySlack, NotifyURL: url, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	mgr, _ := newChannelSecretsManager(t)
	// A crash after the ciphertext write but before the column was cleared.
	if err := mgr.SetValue(ctx, NotificationChannelSecretsKey("chn_1"), channelSecretNotifyURL, "stale-value"); err != nil {
		t.Fatal(err)
	}
	db.SetChannelSecrets(mgr)
	if n, err := db.SealNotificationChannelSecrets(ctx); err != nil || n != 1 {
		t.Fatalf("resume seal = %d, %v", n, err)
	}
	c, _ := db.GetNotificationChannel(ctx, "chn_1")
	r, err := db.RevealNotificationChannel(ctx, *c)
	if err != nil || r.NotifyURL != url {
		t.Fatalf("revealed %q, %v; want the row's own plaintext, not the stale ciphertext", r.NotifyURL, err)
	}
}

func TestChannelSecrets_SurviveMasterKeyRotation(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	mgr, st := newChannelSecretsManager(t)
	db.SetChannelSecrets(mgr)
	ctx := context.Background()
	url := "https://hooks.slack.com/services/" + testSlackToken
	if err := db.SaveNotificationChannel(ctx, slackChannel("chn_1")); err != nil {
		t.Fatal(err)
	}

	newKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RotateMasterKey(ctx, newKey.String()); err != nil {
		t.Fatalf("RotateMasterKey() error = %v", err)
	}
	// A fresh process loading only the new key must still decrypt.
	db.SetChannelSecrets(secrets.NewManager(st, newKey))
	c, _ := db.GetNotificationChannel(ctx, "chn_1")
	r, err := db.RevealNotificationChannel(ctx, *c)
	if err != nil || r.NotifyURL != url || r.InteractiveSecret != testInteractive {
		t.Fatalf("after rotation revealed %+v, %v", r, err)
	}
}

func TestSealedChannel_DispatchPathsDecryptAtSendTime(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	mgr, _ := newChannelSecretsManager(t)
	db.SetChannelSecrets(mgr)
	ctx := context.Background()

	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secretPath := "/hook/" + testSlackToken
	c := NotificationChannel{ID: "chn_1", Name: "S", Kind: NotifySlack, NotifyURL: srv.URL + secretPath, Enabled: true}
	if err := db.SaveNotificationChannel(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	stored, _ := db.GetNotificationChannel(ctx, "chn_1")

	d := NewDeployDispatcher(db, srv.Client(), nil, nil, nil)
	d.Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Succeeded: true})
	if err := d.SendNotice(ctx, *stored, "hello"); err != nil {
		t.Fatalf("SendNotice() error = %v", err)
	}
	if err := d.SendChannelTest(ctx, *stored); err != nil {
		t.Fatalf("SendChannelTest() error = %v", err)
	}
	if err := d.SendApprovalRequest(ctx, *stored, ApprovalRequestInfo{ApprovalID: "apr_1"}); err != nil {
		t.Fatalf("SendApprovalRequest() error = %v", err)
	}

	var captured Rule
	spy := notifierFunc(func(context.Context, Event) error { return nil })
	engine := NewEngine(db, nil, nil, nil, nil, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0,
		func(r Rule) Notifier { captured = r; return spy }, nil)
	_ = engine.sendEvent(ctx, Event{Rule: Rule{ID: "rul_1", ChannelID: "chn_1", NotifyKind: NotifySlack, NotifyURL: stored.NotifyURL}})
	if captured.NotifyURL != srv.URL+secretPath {
		t.Errorf("engine notifier got NotifyURL %q, want the decrypted URL", captured.NotifyURL)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 4 {
		t.Fatalf("server saw %d requests (%v), want 4", len(paths), paths)
	}
	for _, p := range paths {
		if p != secretPath {
			t.Errorf("request path %q, want %q", p, secretPath)
		}
	}
}

func TestPostJSON_RefusesMaskedTarget(t *testing.T) {
	err := sendTextNotification(context.Background(), http.DefaultClient, nil, nil, NotifySlack, "https://hooks.slack.com/(hidden)", "x")
	if !errors.Is(err, ErrChannelSecretNotRevealed) {
		t.Fatalf("send to a masked target err = %v, want ErrChannelSecretNotRevealed", err)
	}
}

type notifierFunc func(context.Context, Event) error

func (f notifierFunc) Notify(ctx context.Context, ev Event) error { return f(ctx, ev) }
