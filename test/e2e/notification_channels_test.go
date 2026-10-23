// Notification channel e2e: a real control plane router with the real
// alerting store and dispatcher, driven over HTTP, delivering to local
// fake vendors. The fakes speak each vendor's documented payload
// contract (JSON shape, auth header, fixed endpoint path), so a passing
// test proves the request this code builds matches that contract. It
// does not prove acceptance by a real vendor account.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

const (
	e2eNotifyAdminUsername = "e2e-notify-admin"
	e2eNotifyAdminPassword = "e2e-notify-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	e2eNotifyFromAddress   = "alerts@example.test"
	e2eNotifyDestination   = "oncall@example.test"
)

type notifyHarness struct {
	t          *testing.T
	baseURL    string
	admin      *http.Client
	alertingDB *alerting.DB
	dispatcher *alerting.DeployDispatcher
	smtp       *fakeSMTP
	push       *recordingPush
	pagerDuty  *fakeVendor
	opsgenie   *fakeVendor
	resend     *fakeVendor
}

func newNotifyHarness(t *testing.T) *notifyHarness {
	t.Helper()
	t.Setenv(netguard.AllowPrivateEnv, "true")
	ctx := context.Background()

	db := openLiveStore(t)
	adb, err := alerting.Open(ctx, filepath.Join(t.TempDir(), "alerting.db"))
	if err != nil {
		t.Fatalf("alerting.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = adb.Close() })

	h := &notifyHarness{
		t: t, alertingDB: adb, smtp: newFakeSMTP(t), push: &recordingPush{},
		pagerDuty: newFakeVendor(t), opsgenie: newFakeVendor(t), resend: newFakeVendor(t),
	}
	sender, err := email.NewSender(email.Config{
		Backend: email.BackendSMTP,
		SMTP:    &email.SMTPConfig{Addr: h.smtp.Addr(), Host: "127.0.0.1", From: e2eNotifyFromAddress, TLSMode: email.TLSNone},
	})
	if err != nil {
		t.Fatalf("email.NewSender() error = %v", err)
	}
	client := &http.Client{Transport: vendorHostRedirect{
		routes: map[string]*fakeVendor{
			"events.pagerduty.com": h.pagerDuty,
			"api.opsgenie.com":     h.opsgenie,
			"api.resend.com":       h.resend,
		},
		base: http.DefaultTransport,
	}}
	h.dispatcher = alerting.NewDeployDispatcher(adb, client, sender, h.push, discardTestLogger())

	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(discardTestLogger(), b, db,
		api.WithNotificationChannels(adb),
		api.WithNotificationChannelTester(h.dispatcher),
		api.WithNotificationDeliveries(adb),
	)
	ts := newE2ETestServer(t, router)
	h.baseURL = ts.URL
	if err := api.BootstrapAdmin(ctx, db, e2eNotifyAdminUsername, e2eNotifyAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	h.admin = loginE2EClient(t, ts.URL, e2eNotifyAdminUsername, e2eNotifyAdminPassword)
	return h
}

type e2eDelivery struct {
	ID      string `json:"id"`
	Trigger string `json:"trigger"`
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func (h *notifyHarness) createChannel(kind, notifyURL string) string {
	h.t.Helper()
	payload, _ := json.Marshal(map[string]any{"name": "e2e-" + kind, "kind": kind, "notify_url": notifyURL})
	status, body := postJSON(h.t, h.admin, h.baseURL+"/api/v1/notification-channels", string(payload))
	if status != http.StatusCreated {
		h.t.Fatalf("create %s channel: status = %d, body = %s", kind, status, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || out.ID == "" {
		h.t.Fatalf("create %s channel: decode %s: %v", kind, body, err)
	}
	return out.ID
}

func (h *notifyHarness) testSend(id string) int {
	h.t.Helper()
	status, _ := postJSON(h.t, h.admin, h.baseURL+"/api/v1/notification-channels/"+id+"/test", "")
	return status
}

func (h *notifyHarness) deliveries(id string) []e2eDelivery {
	h.t.Helper()
	status, raw := requestJSON(h.t, h.admin, http.MethodGet, h.baseURL+"/api/v1/notification-channels/"+id+"/deliveries", "")
	if status != http.StatusOK {
		h.t.Fatalf("list deliveries: status = %d, body = %s", status, raw)
	}
	var out []e2eDelivery
	if err := json.Unmarshal(raw, &out); err != nil {
		h.t.Fatalf("list deliveries: decode %s: %v", raw, err)
	}
	return out
}

func requireString(t *testing.T, m map[string]any, key, want string) {
	t.Helper()
	if got, _ := m[key].(string); got != want {
		t.Errorf("payload[%q] = %q, want %q (payload %v)", key, got, want, m)
	}
}

func requireMarker(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if got, _ := m[key].(string); !strings.Contains(got, "test notification") {
		t.Errorf("payload[%q] = %q, want it to contain the test notification text", key, got)
	}
}

func requireNested(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			t.Fatalf("payload has no object at %q (payload %v)", k, m)
		}
		cur = next
	}
	return cur
}

type httpKindCase struct {
	kind string
	// vendor selects a fixed-host fake; nil means a per-case URL fake.
	vendor func(h *notifyHarness) *fakeVendor
	// target builds notify_url from the fake's base URL.
	target func(base string) string
	check  func(t *testing.T, r fakeVendorRequest)
	// hardOutage also asserts persistent failure reaches delivery history.
	hardOutage bool
}

func httpKindCases() []httpKindCase {
	return []httpKindCase{
		{kind: "generic", target: func(b string) string { return b + "/hook" }, hardOutage: true,
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "message") }},
		{kind: "slack", target: func(b string) string { return b + "/services/T000/B000/XXXX" }, hardOutage: true,
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "text") }},
		{kind: "discord", target: func(b string) string { return b + "/api/webhooks/1/token" }, hardOutage: true,
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "content") }},
		{kind: "pagerduty", vendor: func(h *notifyHarness) *fakeVendor { return h.pagerDuty },
			target: func(string) string { return "0123456789abcdef0123456789abcdef" }, hardOutage: true,
			check: func(t *testing.T, r fakeVendorRequest) {
				if r.Path != "/v2/enqueue" {
					t.Errorf("path = %q, want the Events API v2 path /v2/enqueue", r.Path)
				}
				p := r.JSON(t)
				requireString(t, p, "routing_key", "0123456789abcdef0123456789abcdef")
				requireString(t, p, "event_action", "trigger")
				d := requireNested(t, p, "payload")
				requireMarker(t, d, "summary")
				if d["source"] == "" || d["severity"] == "" {
					t.Errorf("payload.payload = %v, want source and severity, both required by Events API v2", d)
				}
			}},
		{kind: "telegram", target: func(b string) string { return b + "/bot123:abc/sendMessage?chat_id=-1001" },
			check: func(t *testing.T, r fakeVendorRequest) {
				p := r.JSON(t)
				requireString(t, p, "chat_id", "-1001")
				requireMarker(t, p, "text")
			}},
		{kind: "pushover", target: func(b string) string { return b + "/1/messages.json?token=apptoken&user=userkey" },
			check: func(t *testing.T, r fakeVendorRequest) {
				p := r.JSON(t)
				requireString(t, p, "token", "apptoken")
				requireString(t, p, "user", "userkey")
				requireMarker(t, p, "message")
			}},
		{kind: "teams", target: func(b string) string { return b + "/webhookb2/abc" },
			check: func(t *testing.T, r fakeVendorRequest) {
				p := r.JSON(t)
				requireString(t, p, "@type", "MessageCard")
				requireMarker(t, p, "text")
			}},
		{kind: "mattermost", target: func(b string) string { return b + "/hooks/abc" },
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "text") }},
		{kind: "lark", target: func(b string) string { return b + "/open-apis/bot/v2/hook/abc" },
			check: func(t *testing.T, r fakeVendorRequest) {
				p := r.JSON(t)
				requireString(t, p, "msg_type", "text")
				requireMarker(t, requireNested(t, p, "content"), "text")
			}},
		{kind: "rocketchat", target: func(b string) string { return b + "/hooks/abc/def" },
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "text") }},
		{kind: "webex", target: func(b string) string { return b + "/v1/webhooks/incoming/abc" },
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "markdown") }},
		{kind: "googlechat", target: func(b string) string { return b + "/v1/spaces/AAA/messages?key=k&token=t" },
			check: func(t *testing.T, r fakeVendorRequest) { requireMarker(t, r.JSON(t), "text") }},
		{kind: "gotify", target: func(b string) string { return b + "/message?token=apptoken" },
			check: func(t *testing.T, r fakeVendorRequest) {
				requireMarker(t, r.JSON(t), "message")
				if r.Query.Get("token") != "apptoken" {
					t.Errorf("query token = %q, want the Gotify app token preserved", r.Query.Get("token"))
				}
			}},
		{kind: "ntfy", target: func(b string) string { return b + "/ops-alerts?auth=tk_secret" },
			check: func(t *testing.T, r fakeVendorRequest) {
				// ntfy's JSON publish API is the server root with the topic in the body;
				// JSON posted to /topic would be published as literal text.
				if r.Path != "/" {
					t.Errorf("path = %q, want the server root / for ntfy JSON publishing", r.Path)
				}
				p := r.JSON(t)
				requireString(t, p, "topic", "ops-alerts")
				requireMarker(t, p, "message")
				if got := r.Header.Get("Authorization"); got != "Bearer tk_secret" {
					t.Errorf("Authorization = %q, want Bearer tk_secret moved out of the URL", got)
				}
				if r.Query.Get("auth") != "" {
					t.Errorf("auth token leaked in the query string")
				}
			}},
		{kind: "opsgenie", vendor: func(h *notifyHarness) *fakeVendor { return h.opsgenie },
			target: func(string) string { return "https://api.opsgenie.com/v2/alerts?key=genie-key" },
			check: func(t *testing.T, r fakeVendorRequest) {
				if r.Path != "/v2/alerts" {
					t.Errorf("path = %q, want /v2/alerts", r.Path)
				}
				if got := r.Header.Get("Authorization"); got != "GenieKey genie-key" {
					t.Errorf("Authorization = %q, want GenieKey genie-key", got)
				}
				p := r.JSON(t)
				if p["message"] == "" {
					t.Errorf("payload = %v, want the required message field", p)
				}
				requireMarker(t, p, "description")
			}},
		{kind: "resend", vendor: func(h *notifyHarness) *fakeVendor { return h.resend },
			target: func(string) string { return "https://api.resend.com/emails?key=re_key&to=" + e2eNotifyDestination },
			check: func(t *testing.T, r fakeVendorRequest) {
				if r.Path != "/emails" {
					t.Errorf("path = %q, want /emails", r.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer re_key" {
					t.Errorf("Authorization = %q, want Bearer re_key", got)
				}
				p := r.JSON(t)
				to, _ := p["to"].([]any)
				if len(to) != 1 || to[0] != e2eNotifyDestination {
					t.Errorf("payload.to = %v, want [%s]", p["to"], e2eNotifyDestination)
				}
				if p["from"] == "" || p["subject"] == "" {
					t.Errorf("payload = %v, want from and subject, both required by the Resend API", p)
				}
				requireMarker(t, p, "text")
			}},
	}
}

func TestNotificationChannels_Live_VendorContractsRetryAndHistory(t *testing.T) {
	h := newNotifyHarness(t)

	for _, tc := range httpKindCases() {
		t.Run(tc.kind, func(t *testing.T) {
			h.t = t
			fake := newFakeVendor(t)
			if tc.vendor != nil {
				fake = tc.vendor(h)
				fake.reset()
			}
			id := h.createChannel(tc.kind, tc.target(fake.URL()))

			if status := h.testSend(id); status != http.StatusNoContent {
				t.Fatalf("test send: status = %d, want %d", status, http.StatusNoContent)
			}
			reqs := fake.snapshot()
			if len(reqs) != 1 {
				t.Fatalf("vendor saw %d requests for one send, want 1", len(reqs))
			}
			if reqs[0].Method != http.MethodPost {
				t.Errorf("method = %s, want POST", reqs[0].Method)
			}
			if ct := reqs[0].Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			tc.check(t, reqs[0])

			fake.reset()
			fake.failNext.Store(1)
			if status := h.testSend(id); status != http.StatusNoContent {
				t.Fatalf("send after one transient 503: status = %d, want it to retry and succeed", status)
			}
			if got := len(fake.snapshot()); got != 2 {
				t.Errorf("vendor saw %d requests across a transient failure, want 2 (one failure, one retry)", got)
			}

			want := []bool{true, true}
			if tc.hardOutage {
				fake.reset()
				fake.failAlways.Store(true)
				if status := h.testSend(id); status != http.StatusBadGateway {
					t.Fatalf("send during a hard outage: status = %d, want %d", status, http.StatusBadGateway)
				}
				if got := len(fake.snapshot()); got != 3 {
					t.Errorf("vendor saw %d requests during a hard outage, want exactly 3 attempts", got)
				}
				want = []bool{false, true, true}
			}

			got := h.deliveries(id)
			if len(got) != len(want) {
				t.Fatalf("delivery history = %+v, want %d entries newest first", got, len(want))
			}
			for i, w := range want {
				if got[i].Success != w || got[i].Trigger != "test" {
					t.Errorf("delivery[%d] = %+v, want success=%t trigger=test", i, got[i], w)
				}
			}
			if tc.hardOutage && !strings.Contains(got[0].Error, "503") {
				t.Errorf("failed delivery error = %q, want it to carry the vendor status", got[0].Error)
			}
		})
	}

	t.Run("email over SMTP", func(t *testing.T) {
		h.t = t
		id := h.createChannel("email", e2eNotifyDestination)
		if status := h.testSend(id); status != http.StatusNoContent {
			t.Fatalf("test send: status = %d, want %d", status, http.StatusNoContent)
		}
		msgs := h.smtp.received()
		if len(msgs) != 1 || msgs[0].To != e2eNotifyDestination || msgs[0].From != e2eNotifyFromAddress {
			t.Fatalf("smtp received %+v, want one message from %s to %s", msgs, e2eNotifyFromAddress, e2eNotifyDestination)
		}
		if !strings.Contains(msgs[0].Data, "Subject:") || !strings.Contains(msgs[0].Data, "test notification") {
			t.Errorf("smtp message = %q, want a Subject header and the test text", msgs[0].Data)
		}

		h.smtp.rcptAttempts.Store(0)
		h.smtp.failRcptNext.Store(1)
		if status := h.testSend(id); status != http.StatusNoContent {
			t.Fatalf("send after a transient 421: status = %d, want it to retry and succeed", status)
		}
		if got := h.smtp.rcptAttempts.Load(); got != 2 {
			t.Errorf("smtp RCPT attempts = %d across a transient failure, want 2", got)
		}
		if got := len(h.smtp.received()); got != 2 {
			t.Errorf("smtp received %d messages, want 2 (the retried one delivered once)", got)
		}

		h.smtp.rcptAttempts.Store(0)
		h.smtp.failRcptNext.Store(100)
		if status := h.testSend(id); status != http.StatusBadGateway {
			t.Fatalf("send during an SMTP outage: status = %d, want %d", status, http.StatusBadGateway)
		}
		h.smtp.failRcptNext.Store(0)
		if got := h.smtp.rcptAttempts.Load(); got != 3 {
			t.Errorf("smtp RCPT attempts = %d during an outage, want exactly 3", got)
		}
		got := h.deliveries(id)
		if len(got) != 3 || got[0].Success || !got[1].Success || !got[2].Success {
			t.Errorf("delivery history = %+v, want [failed, ok, ok] newest first", got)
		}
	})

	t.Run("webpush through a recording sender", func(t *testing.T) {
		h.t = t
		id := h.createChannel("webpush", "")
		if status := h.testSend(id); status != http.StatusNoContent {
			t.Fatalf("test send: status = %d, want %d", status, http.StatusNoContent)
		}
		if got := h.push.count(); got != 1 {
			t.Errorf("push sender saw %d sends, want 1", got)
		}
		if got := h.deliveries(id); len(got) != 1 || !got[0].Success {
			t.Errorf("delivery history = %+v, want one success", got)
		}
	})
}

// TestNotificationChannels_Live_DeployOutcomeDispatch covers the real
// send path (not the operator test button): a failed deploy reaches the
// channel attached to the app, retries a transient outage, and lands in
// delivery history with the deploy trigger.
func TestNotificationChannels_Live_DeployOutcomeDispatch(t *testing.T) {
	h := newNotifyHarness(t)
	ctx := context.Background()
	slack := newFakeVendor(t)
	slackID := h.createChannel("slack", slack.URL()+"/services/T000/B000/XXXX")
	pdID := h.createChannel("pagerduty", "0123456789abcdef0123456789abcdef")

	const resource = "service:checkout"
	targets := []alerting.DeployTarget{
		{ID: "dnt_e2e_slack", ResourceID: resource, ChannelID: slackID, NotifyKind: alerting.NotifySlack, Enabled: true},
		{ID: "dnt_e2e_pd", ResourceID: resource, ChannelID: pdID, NotifyKind: alerting.NotifyPagerDuty, NotifyURL: "0123456789abcdef0123456789abcdef", Enabled: true},
	}
	for _, tg := range targets {
		if err := h.alertingDB.SaveDeployTarget(ctx, tg); err != nil {
			t.Fatalf("SaveDeployTarget(%s) error = %v", tg.ID, err)
		}
	}

	slack.failNext.Store(1)
	h.dispatcher.Dispatch(ctx, resource, alerting.DeployOutcome{AppName: "checkout", Image: "shop/checkout:v9", Succeeded: false, Error: "readiness probe failed"})

	reqs := slack.snapshot()
	if len(reqs) != 2 {
		t.Fatalf("slack saw %d requests, want 2 (a 503 then the retry)", len(reqs))
	}
	text, _ := reqs[1].JSON(t)["text"].(string)
	if !strings.Contains(text, "checkout") || !strings.Contains(text, "readiness probe failed") {
		t.Errorf("slack text = %q, want the app name and the failure reason", text)
	}
	pd := h.pagerDuty.snapshot()
	if len(pd) != 1 {
		t.Fatalf("pagerduty saw %d requests, want 1", len(pd))
	}
	details := requireNested(t, pd[0].JSON(t), "payload")
	if details["severity"] != "critical" {
		t.Errorf("pagerduty severity = %v, want critical for a failed deploy", details["severity"])
	}

	for _, id := range []string{slackID, pdID} {
		got := h.deliveries(id)
		if len(got) != 1 || !got[0].Success || got[0].Trigger != "deploy-failed" {
			t.Errorf("channel %s delivery history = %+v, want one successful deploy-failed entry", id, got)
		}
	}

	// A disabled channel target is skipped: no extra request, no history row.
	disabled := targets[0]
	disabled.Enabled = false
	if err := h.alertingDB.SaveDeployTarget(ctx, disabled); err != nil {
		t.Fatalf("SaveDeployTarget(disabled) error = %v", err)
	}
	before := len(slack.snapshot())
	h.dispatcher.Dispatch(ctx, resource, alerting.DeployOutcome{AppName: "checkout", Image: "shop/checkout:v10", Succeeded: true})
	if after := len(slack.snapshot()); after != before {
		t.Errorf("disabled slack target received %d new requests, want 0", after-before)
	}
}
