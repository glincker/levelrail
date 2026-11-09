package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

var channelFixtureSecrets = []string{"FIXTURE-SLACK-TOKEN", "fixture-bot-token", "fixture-secret-topic", "fixture-ntfy-token", "fixture-smtp-password"}

// channelsDiscovery mirrors internal/platformimport/testdata/dokploy/notifications.json.
func channelsDiscovery() *platformimport.Discovery {
	return &platformimport.Discovery{Notifications: []platformimport.NotificationProvider{
		{SourceID: "ntf-slack", Name: "Ops Slack", Kind: "slack", HasSecret: true},
		{SourceID: "ntf-tg", Name: "Pager Telegram", Kind: "telegram", HasSecret: true},
		{SourceID: "ntf-ntfy", Name: "Phone", Kind: "ntfy", Server: "https://ntfy.example.com", HasSecret: true, HasAccessToken: true},
		{SourceID: "ntf-mail", Name: "On-call Mail", Kind: "email", Recipient: "oncall@example.com"},
		{SourceID: "ntf-matrix", Name: "Matrix Room", Kind: "matrix", HasSecret: true},
	}}
}

var fixtureTargets = map[string]platformimport.NotifyTarget{
	"ntf-slack": {Kind: "slack", URL: "https://hooks.example.com/services/FIXTURE-SLACK-TOKEN"},
	"ntf-tg":    {Kind: "telegram", URL: "https://api.telegram.org/botfixture-bot-token/sendMessage?chat_id=-1001"},
	"ntf-ntfy":  {Kind: "ntfy", Server: "https://ntfy.example.com", URL: "https://ntfy.example.com/fixture-secret-topic?auth=fixture-ntfy-token"},
}

type fakeChannelAPI struct {
	mu       sync.Mutex
	channels []apiclient.NotificationChannelResource
	created  []createNotificationChannelRequest
	deleted  []string
	failWith string
	// forceEnabled makes create ignore a disabled request.
	forceEnabled bool
}

func (f *fakeChannelAPI) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/notification-channels":
		_ = json.NewEncoder(w).Encode(f.channels)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/notification-channels":
		var req createNotificationChannelRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if f.failWith != "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": f.failWith + req.NotifyURL})
			return
		}
		f.created = append(f.created, req)
		ch := apiclient.NotificationChannelResource{ID: "ch-" + req.Name, Name: req.Name, Kind: req.Kind, Enabled: f.forceEnabled || (req.Enabled != nil && *req.Enabled), CreatedAt: "2026-11-09T00:00:00Z", UpdatedAt: "2026-11-09T00:00:00Z"}
		f.channels = append(f.channels, ch)
		_ = json.NewEncoder(w).Encode(ch)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/notification-channels/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/notification-channels/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}
}

func newFakeChannelAPI(t *testing.T, api *fakeChannelAPI) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(api.handler))
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "tok")
}

func planChannelsForTest(useSecrets bool) *channelImport {
	imp := planDokployChannels(channelsDiscovery(), channelPlanOptions{limit: 50, checkURL: func(string) error { return nil }})
	imp.finishPlan(useSecrets)
	if useSecrets {
		imp.target = func(_ context.Context, id string) (platformimport.NotifyTarget, error) {
			return fixtureTargets[id], nil
		}
	}
	return imp
}

// assertNoChannelSecret checks the JSON report, the human output and the journal file.
func assertNoChannelSecret(t *testing.T, imp *channelImport, jr *applyJournal) {
	t.Helper()
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(imp)
	printChannelImport(&buf, imp)
	if jr != nil {
		if b, err := os.ReadFile(jr.path); err == nil {
			buf.Write(b)
		}
	}
	for _, s := range channelFixtureSecrets {
		if strings.Contains(buf.String(), s) {
			t.Fatalf("output leaks %q:\n%s", s, buf.String())
		}
	}
}

func TestDokployChannelMapKinds(t *testing.T) {
	imp := planChannelsForTest(false)
	want := map[string]string{"ops-slack": "slack", "pager-telegram": "telegram", "phone": "ntfy", "on-call-mail": "email"}
	if len(imp.Channels) != 4 {
		t.Fatalf("got %d channels, want 4: %+v", len(imp.Channels), imp.Channels)
	}
	for _, c := range imp.Channels {
		if want[c.Name] != c.Kind || c.Enabled {
			t.Fatalf("channel %+v, want kind %q disabled", c, want[c.Name])
		}
	}
	if len(imp.Unsupported) != 1 || imp.Unsupported[0].SourceKind != "matrix" || imp.Unsupported[0].Reason == "" {
		t.Fatalf("unsupported = %+v", imp.Unsupported)
	}
}

func TestDokployChannelNeedsSecretHintHasPlaceholdersOnly(t *testing.T) {
	imp := planChannelsForTest(false)
	for _, c := range imp.Channels {
		if c.Kind == "email" {
			if c.State != backupWouldCreate {
				t.Fatalf("email needs no secret: %+v", c)
			}
			continue
		}
		if c.State != backupNeedsSecret || !strings.Contains(c.Detail, "--name '"+c.Name+"'") || !strings.Contains(c.Detail, "<") || !strings.HasSuffix(c.Detail, "--disabled") {
			t.Fatalf("hint for %s = %q", c.Name, c.Detail)
		}
	}
	assertNoChannelSecret(t, imp, nil)
}

func TestDokployChannelImportCreatesDisabledWithoutLeaking(t *testing.T) {
	api := &fakeChannelAPI{}
	client := newFakeChannelAPI(t, api)
	jr := testJournal(t)
	imp := planChannelsForTest(true)
	applyChannelImport(context.Background(), client, imp, jr)
	if len(api.created) != 4 {
		t.Fatalf("created %d channels, want 4: %+v", len(api.created), imp.Channels)
	}
	for _, req := range api.created {
		if req.Enabled == nil || *req.Enabled {
			t.Fatalf("channel %s not created disabled", req.Name)
		}
		if req.Kind == "email" && req.NotifyURL != "oncall@example.com" {
			t.Fatalf("email target = %q", req.NotifyURL)
		}
	}
	for _, c := range imp.Channels {
		r := jr.findCreated(journalKindChannel, c.Name)
		if c.State != backupCreated || r == nil || r.TargetID != c.ID || r.Fingerprint == "" {
			t.Fatalf("channel %+v journal %+v", c, r)
		}
	}
	assertNoChannelSecret(t, imp, jr)
}

func TestDokployChannelNameCollisionSkips(t *testing.T) {
	api := &fakeChannelAPI{channels: []apiclient.NotificationChannelResource{{ID: "mine", Name: "ops-slack", Kind: "slack"}}}
	imp := planChannelsForTest(true)
	applyChannelImport(context.Background(), newFakeChannelAPI(t, api), imp, testJournal(t))
	for _, req := range api.created {
		if req.Name == "ops-slack" {
			t.Fatal("existing channel overwritten")
		}
	}
	if imp.Channels[0].State != backupExists || !strings.Contains(imp.Channels[0].Detail, "warning") {
		t.Fatalf("collision = %+v", imp.Channels[0])
	}
}

func TestDokployChannelCreateErrorIsRedacted(t *testing.T) {
	api := &fakeChannelAPI{failWith: "invalid notify_url: "}
	imp := planChannelsForTest(true)
	applyChannelImport(context.Background(), newFakeChannelAPI(t, api), imp, testJournal(t))
	for _, c := range imp.Channels {
		if c.State != backupFailed {
			t.Fatalf("channel %+v should fail", c)
		}
	}
	assertNoChannelSecret(t, imp, nil)
}

func TestDokployChannelPolicyRefusalSkipsCreate(t *testing.T) {
	api := &fakeChannelAPI{}
	imp := planChannelsForTest(true)
	imp.checkURL = func(string) error {
		return platformimport.ValidateNotifyEndpoint(context.Background(), "http://x", platformimport.NetworkPolicy{})
	}
	applyChannelImport(context.Background(), newFakeChannelAPI(t, api), imp, nil)
	if len(api.created) != 1 || api.created[0].Kind != "email" {
		t.Fatalf("only the email channel may be created, got %+v", api.created)
	}
}

func TestDokployChannelUndoMatchesFingerprint(t *testing.T) {
	ch := apiclient.NotificationChannelResource{ID: "ch-1", Name: "ops-slack", Kind: "slack", CreatedAt: "2026-11-09T00:00:00Z"}
	r := &journalResource{Kind: journalKindChannel, Name: "ops-slack", CreatedByRun: true, TargetID: "ch-1", Fingerprint: channelFingerprint(ch)}
	ctx := context.Background()
	if action, reason := planChannelUndo(ctx, newFakeChannelAPI(t, &fakeChannelAPI{}), r); action != undoSkip || !strings.Contains(reason, "cannot tell") {
		t.Fatalf("empty list: %s %s", action, reason)
	}
	other := apiclient.NotificationChannelResource{ID: "ch-2", Name: "x", Kind: "slack"}
	if action, reason := planChannelUndo(ctx, newFakeChannelAPI(t, &fakeChannelAPI{channels: []apiclient.NotificationChannelResource{other}}), r); action != undoSkip || !strings.Contains(reason, "cannot tell") {
		t.Fatalf("missing id: %s %s", action, reason)
	}
	renamed := ch
	renamed.Name = "renamed"
	if action, _ := planChannelUndo(ctx, newFakeChannelAPI(t, &fakeChannelAPI{channels: []apiclient.NotificationChannelResource{renamed}}), r); action != undoSkip {
		t.Fatal("changed channel must not be removed")
	}
	if action, _ := planChannelUndo(ctx, newFakeChannelAPI(t, &fakeChannelAPI{channels: []apiclient.NotificationChannelResource{ch}}), r); action != undoRemove {
		t.Fatal("matching channel should be removed")
	}
}

func TestDokployChannelHostileNamesGoToReview(t *testing.T) {
	d := &platformimport.Discovery{Notifications: []platformimport.NotificationProvider{
		{SourceID: "a", Name: "!!!", Kind: "slack"},
		{SourceID: "b", Name: strings.Repeat("a", 64), Kind: "slack"},
		{SourceID: "c", Name: "hdr", Kind: "custom", HasHeaders: true},
		{SourceID: "e", Name: "mixed", Kind: "slack", Mismatch: true},
		{SourceID: "d", Name: "bad-ntfy", Kind: "ntfy", Server: "https://ntfy.example.com/?topic=x"},
	}}
	imp := planDokployChannels(d, channelPlanOptions{limit: 50})
	for _, c := range imp.Channels {
		if c.State != backupReview || c.Server != "" {
			t.Fatalf("%+v should go to review", c)
		}
	}
}

func TestDokployChannelUndoLeavesEditedChannel(t *testing.T) {
	api := &fakeChannelAPI{}
	client := newFakeChannelAPI(t, api)
	jr := testJournal(t)
	applyChannelImport(context.Background(), client, planChannelsForTest(true), jr)
	r := jr.findCreated(journalKindChannel, "ops-slack")
	api.channels[0].UpdatedAt = "2026-11-10T00:00:00Z"
	if action, reason := planChannelUndo(context.Background(), client, r); action != undoSkip || !strings.Contains(reason, "changed") {
		t.Fatalf("edited channel: %s %s", action, reason)
	}
}

func TestDokployChannelServerEnabledIsDeleted(t *testing.T) {
	api := &fakeChannelAPI{forceEnabled: true}
	jr := testJournal(t)
	imp := planChannelsForTest(true)
	applyChannelImport(context.Background(), newFakeChannelAPI(t, api), imp, jr)
	if len(api.deleted) != 4 {
		t.Fatalf("deleted %v, want all 4 enabled channels", api.deleted)
	}
	for _, c := range imp.Channels {
		if c.State != backupFailed || jr.findCreated(journalKindChannel, c.Name) != nil {
			t.Fatalf("channel %+v should be failed and not journaled as created", c)
		}
	}
}

func TestDokployChannelSourceChangedSincePlan(t *testing.T) {
	api := &fakeChannelAPI{}
	imp := planChannelsForTest(true)
	imp.target = func(_ context.Context, id string) (platformimport.NotifyTarget, error) {
		tgt := fixtureTargets[id]
		if tgt.Kind == "ntfy" {
			tgt.Server = "https://elsewhere.example.com"
		}
		if tgt.Kind == "slack" {
			tgt.Kind = "discord"
		}
		return tgt, nil
	}
	applyChannelImport(context.Background(), newFakeChannelAPI(t, api), imp, nil)
	for _, c := range imp.Channels {
		if (c.Kind == "ntfy" || c.Kind == "slack") && (c.State != backupReview || !strings.Contains(c.Detail, "source changed")) {
			t.Fatalf("channel %+v should go to review", c)
		}
	}
	if len(api.created) != 2 {
		t.Fatalf("created %d, want telegram and email only", len(api.created))
	}
}

func TestCleanDetailStripsFormatChars(t *testing.T) {
	s := "ok\u202eevil\u200b"
	if !hasControlChar(s) || strings.ContainsAny(cleanDetail(s), "\u202e\u200b") {
		t.Fatalf("format characters kept: %q", cleanDetail(s))
	}
}
