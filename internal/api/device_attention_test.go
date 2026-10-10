package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeviceVerificationBase(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		remoteAddr string
		headers    map[string]string
		env        map[string]string
		want       string
	}{
		{"ssh tunnel on a custom port", "127.0.0.1:28080", "127.0.0.1:50000", nil, nil, "http://127.0.0.1:28080"},
		{"plain host", "levelrail.example.com", "203.0.113.7:4000", nil, nil, "http://levelrail.example.com"},
		{"proxy on loopback forwards host and proto", "127.0.0.1:8080", "127.0.0.1:50000",
			map[string]string{"X-Forwarded-Host": "deploy.example.com", "X-Forwarded-Proto": "https"}, nil, "https://deploy.example.com"},
		{"forwarded host from public peer is ignored", "10.0.0.5:8080", "198.51.100.9:4000",
			map[string]string{"X-Forwarded-Host": "evil.example.com"}, nil, "http://10.0.0.5:8080"},
		{"forwarded host from configured proxy cidr", "10.0.0.5:8080", "172.18.0.2:4000",
			map[string]string{"X-Forwarded-Host": "deploy.example.com", "X-Forwarded-Proto": "https"},
			map[string]string{envTrustedProxies: "172.18.0.0/16"}, "https://deploy.example.com"},
		{"forwarded host with a path is ignored", "127.0.0.1:8080", "127.0.0.1:50000",
			map[string]string{"X-Forwarded-Host": "evil.example.com/phish"}, nil, "http://127.0.0.1:8080"},
		{"unknown forwarded proto is ignored", "127.0.0.1:8080", "127.0.0.1:50000",
			map[string]string{"X-Forwarded-Proto": "javascript"}, nil, "http://127.0.0.1:8080"},
		{"explicit dashboard url wins", "127.0.0.1:28080", "127.0.0.1:50000", nil,
			map[string]string{envDashboardURL: "https://panel.example.com/"}, "https://panel.example.com"},
		{"invalid dashboard url is ignored", "127.0.0.1:28080", "127.0.0.1:50000", nil,
			map[string]string{envDashboardURL: "not a url"}, "http://127.0.0.1:28080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envDashboardURL, "")
			t.Setenv(envTrustedProxies, "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", nil)
			r.Host = tt.host
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := deviceVerificationBase(r); got != tt.want {
				t.Errorf("deviceVerificationBase = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeviceStart_URLFollowsRequestHost(t *testing.T) {
	t.Setenv(envDashboardURL, "")
	rt, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:28080/api/v1/auth/device/start", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	var got deviceStartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v, body = %s", err, rec.Body.String())
	}
	want := "http://127.0.0.1:28080/settings/cli-access?user_code=" + got.UserCode
	if got.VerificationURIComplete != want {
		t.Errorf("VerificationURIComplete = %q, want %q", got.VerificationURIComplete, want)
	}
	if got.VerificationURI != "http://127.0.0.1:28080/settings/cli-access" {
		t.Errorf("VerificationURI = %q", got.VerificationURI)
	}
}

func TestDeviceDecide_RejectsAPITokens(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	started := startDeviceAuth(t, rt, "laptop")
	tok := mintTokenAs(t, rt, cookie, "agent")

	for _, action := range []string{"approve", "deny"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/"+action, "", tok.Token))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with an API token: status = %d, want %d", action, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestDevicePendingSummary_NeverExposesCodes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	started := startDeviceAuth(t, rt, "laptop")
	tok := mintTokenAs(t, rt, cookie, "agent")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/auth/device/pending-summary", "", tok.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, started.UserCode) || strings.Contains(body, started.DeviceCode) {
		t.Errorf("summary leaked a code: %s", body)
	}
	var got devicePendingSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pending) != 1 || got.Pending[0].ClientName != "laptop" {
		t.Errorf("pending = %+v, want the one laptop request", got.Pending)
	}

	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/pending-summary", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Errorf("anonymous status = %d, want %d", anon.Code, http.StatusUnauthorized)
	}
}

func TestDeviceList_FlagsRequesterIPMismatch(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	startDeviceAuth(t, rt, "laptop")

	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{"same ip", "192.0.2.1:1234", false},
		{"different ip", "203.0.113.50:1234", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/device/requests", "")
			req.RemoteAddr = tt.remoteAddr
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			var got []deviceAuthRequestResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 {
				t.Fatalf("decode: %v, got = %+v", err, got)
			}
			if got[0].RequesterIP != "192.0.2.1" || got[0].IPMismatch != tt.want {
				t.Errorf("got %+v, want requester 192.0.2.1 mismatch=%v", got[0], tt.want)
			}
		})
	}
}

func TestDeviceDecisions_AreAudited(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	approved := startDeviceAuth(t, rt, "one")
	denied := startDeviceAuth(t, rt, "two")

	for _, step := range []struct{ code, action string }{{approved.UserCode, "approve"}, {denied.UserCode, "deny"}} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+step.code+"/"+step.action, ""))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d", step.action, rec.Code)
		}
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Path, "/approve"):
			seen["approve"] = true
		case strings.HasSuffix(e.Path, "/deny"):
			seen["deny"] = true
		}
		if strings.Contains(e.Path, approved.DeviceCode) || strings.Contains(e.Path, denied.DeviceCode) {
			t.Errorf("audit path leaked a device code: %s", e.Path)
		}
	}
	if !seen["approve"] || !seen["deny"] {
		t.Errorf("audit decisions seen = %v, want approve and deny", seen)
	}
}

type recordingNotifier struct {
	mu    sync.Mutex
	texts []string
	done  chan struct{}
}

func (n *recordingNotifier) SendNotice(_ context.Context, _ alerting.NotifyKind, _, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.texts = append(n.texts, text)
	select {
	case n.done <- struct{}{}:
	default:
	}
	return nil
}

func TestDeviceStart_NotifiesOnlyOptedInChannelsWithoutCode(t *testing.T) {
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	notifier := &recordingNotifier{done: make(chan struct{}, 4)}
	rt := NewRouter(nil, testBrand(), db,
		WithNotificationChannels(adb), WithDeviceLoginNotifier(notifier), WithDashboardURL("https://panel.example.com"))

	ctx := context.Background()
	for i, optIn := range []bool{true, false} {
		id, err := alerting.NewNotificationChannelID()
		if err != nil {
			t.Fatal(err)
		}
		err = adb.SaveNotificationChannel(ctx, alerting.NotificationChannel{
			ID: id, Name: "c" + string(rune('a'+i)), Kind: alerting.NotifyGeneric,
			NotifyURL: "https://example.com/hook", Enabled: true, NotifyDeviceLogin: optIn,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	started := startDeviceAuth(t, rt, "laptop")
	select {
	case <-notifier.done:
	case <-time.After(3 * time.Second):
		t.Fatal("no notice sent to the opted-in channel")
	}
	// A second start inside the gate interval must not send again.
	startDeviceAuth(t, rt, "laptop2")
	time.Sleep(100 * time.Millisecond)

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.texts) != 1 {
		t.Fatalf("notices = %d, want exactly 1 (one opted-in channel, gated)", len(notifier.texts))
	}
	text := notifier.texts[0]
	if strings.Contains(text, started.UserCode) || strings.Contains(text, started.DeviceCode) {
		t.Errorf("notice leaked a code: %q", text)
	}
	if !strings.Contains(text, "https://panel.example.com/settings/cli-access") {
		t.Errorf("notice = %q, want the configured dashboard link", text)
	}
}

func TestDeviceNoticeGate(t *testing.T) {
	var g deviceNoticeGate
	now := time.Now()
	tests := []struct {
		at   time.Duration
		want bool
	}{
		{0, true}, {10 * time.Second, false}, {deviceNoticeInterval + time.Second, true},
	}
	for _, tt := range tests {
		if got := g.take(now.Add(tt.at)); got != tt.want {
			t.Errorf("take(+%v) = %v, want %v", tt.at, got, tt.want)
		}
	}
}
