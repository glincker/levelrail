package alerting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type recordingPlugins struct {
	name, secret string
	notice       PluginNotice
	err          error
}

func (r *recordingPlugins) SendPlugin(_ context.Context, plugin, secret string, n PluginNotice) error {
	r.name, r.secret, r.notice = plugin, secret, n
	return r.err
}

func TestPluginChannelKind(t *testing.T) {
	if !IsPluginKind("plugin:example-notify") || IsPluginKind(NotifySlack) || IsPluginKind("") {
		t.Fatal("IsPluginKind misclassified a kind")
	}
	if got := PluginName("plugin:example-notify"); got != "example-notify" {
		t.Fatalf("PluginName = %q", got)
	}
}

func TestPluginNotifierDeliversThroughThePluginOnly(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	rp := &recordingPlugins{}
	rule := Rule{ID: "r1", Name: "disk", Kind: KindThreshold, ResourceID: "app:web", NotifyKind: "plugin:example-notify", NotifyURL: "tok-1", Severity: "critical"}
	n := NewNotifierWithPlugins(srv.Client(), nil, nil, rp, rule)
	ev := Event{Rule: rule, LogLines: []string{"password=hunter2"}}
	if err := n.Notify(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if rp.name != "example-notify" || rp.secret != "tok-1" || rp.notice.Status != "firing" || rp.notice.Severity != "critical" {
		t.Fatalf("plugin call = %+v", rp)
	}
	if strings.Contains(rp.notice.Body, "hunter2") || len(rp.notice.LogLines) != 1 {
		t.Fatalf("log lines must travel only in LogLines so the runtime can gate them, body = %q", rp.notice.Body)
	}

	ev.Resolved = true
	if err := n.Notify(context.Background(), ev); err != nil || rp.notice.Status != "resolved" || len(rp.notice.LogLines) != 0 {
		t.Fatalf("resolved notice = %+v, %v", rp.notice, err)
	}
	if hits.Load() != 0 {
		t.Fatal("a plugin channel made an HTTP call")
	}
}

func TestPluginKindNeverFallsBackToGenericWebhook(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	rule := Rule{Name: "x", NotifyKind: "plugin:gone", NotifyURL: srv.URL}

	if err := NewNotifier(srv.Client(), nil, nil, rule).Notify(context.Background(), Event{Rule: rule}); err == nil {
		t.Fatal("plugin kind through the plain constructor must fail, not post the secret")
	}
	if err := NewNotifierWithPlugins(srv.Client(), nil, nil, nil, rule).Notify(context.Background(), Event{Rule: rule}); !errors.Is(err, ErrPluginUnavailable) {
		t.Fatalf("no plugin runtime = %v, want ErrPluginUnavailable", err)
	}
	if err := sendTextNotification(context.Background(), srv.Client(), nil, nil, "plugin:gone", srv.URL, "t"); !errors.Is(err, ErrPluginChannelUnsupported) {
		t.Fatalf("test send = %v", err)
	}
	if err := sendDeployOutcome(context.Background(), srv.Client(), nil, nil, DeployTarget{NotifyKind: "plugin:gone", NotifyURL: srv.URL}, DeployOutcome{}); !errors.Is(err, ErrPluginChannelUnsupported) {
		t.Fatalf("deploy outcome = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the credential was posted to a URL")
	}
}
