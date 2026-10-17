package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func runningHarness(t *testing.T, approve []string) *harness {
	t.Helper()
	h := newHarness(t)
	h.installApprovedEnabled("[read, read:sensitive]", approve)
	h.setDescribe("example", []string{AbilityRead})
	h.m.Reconcile(context.Background())
	if h.m.inst("example").connected() == nil {
		t.Fatal("plugin did not start")
	}
	return h
}

func TestSendPassesOnlyEventConfigAndPerCallSecret(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	d, err := h.m.Send(context.Background(), "example", "s3cret-token", Notification{ID: "e1", Title: "disk full", Body: "98%", Status: "firing"})
	if err != nil || !d.Delivered {
		t.Fatalf("Send = %+v, %v", d, err)
	}
	got := h.rpc.sent[0]
	if got.GetSecret() != "s3cret-token" || got.GetEvent().GetTitle() != "disk full" || got.GetConfigJson() != "{}" {
		t.Fatalf("request = %v", got)
	}
	rec, _ := h.store.GetPlugin(context.Background(), "example")
	if strings.Contains(rec.ConfigJSON+rec.Manifest+rec.LastError, "s3cret-token") {
		t.Fatal("the per-call secret was persisted")
	}
	for _, e := range h.audit.events {
		for _, v := range e.Detail {
			if strings.Contains(v, "s3cret-token") {
				t.Fatal("the per-call secret reached the audit log")
			}
		}
	}
}

func TestSendAuditsEveryAbilityUse(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead, AbilityReadSensitive})
	for range 2 {
		if _, err := h.m.Send(context.Background(), "example", "", Notification{ID: "e", Title: "t", SensitiveLines: []string{"line"}}); err != nil {
			t.Fatal(err)
		}
	}
	var uses []AuditEvent
	for _, e := range h.audit.events {
		if e.Action == AuditAbilityUsed {
			uses = append(uses, e)
		}
	}
	if len(uses) != 2 {
		t.Fatalf("ability-use audit entries = %d, want 2", len(uses))
	}
	if uses[0].Detail["abilities"] != "read,read:sensitive" || uses[0].Actor != "plugin:example" {
		t.Fatalf("entry = %+v", uses[0])
	}
}

func TestSensitiveLinesNeedTheSensitiveApproval(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	if _, err := h.m.Send(context.Background(), "example", "", Notification{Title: "t", Body: "b", SensitiveLines: []string{"password=hunter2"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h.rpc.sent[0].GetEvent().GetBody(), "hunter2") {
		t.Fatal("log lines reached a plugin that was not approved for read:sensitive")
	}

	h2 := runningHarness(t, []string{AbilityRead, AbilityReadSensitive})
	if _, err := h2.m.Send(context.Background(), "example", "", Notification{Title: "t", Body: "b", SensitiveLines: []string{"line-1"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h2.rpc.sent[0].GetEvent().GetBody(), "line-1") {
		t.Fatal("approved read:sensitive did not deliver the lines")
	}
}

func TestSendGating(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	if _, err := h.m.Install(ctx, h.request(manifestYAML("[read]")), "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Send(ctx, "example", "", Notification{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("send to a plugin that is not enabled = %v", err)
	}
	if _, err := h.m.Send(ctx, "missing", "", Notification{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("send to an unknown plugin = %v", err)
	}

	h2 := runningHarness(t, []string{AbilityRead})
	if _, err := h2.m.Disable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h2.m.Send(ctx, "example", "", Notification{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("send after disable = %v", err)
	}
}

func TestPayloadBounds(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	huge := strings.Repeat("x", 1<<20)
	if _, err := h.m.Send(context.Background(), "example", "", Notification{Title: huge, Body: huge, Link: huge, ResourceID: huge}); err != nil {
		t.Fatal(err)
	}
	ev := h.rpc.sent[0].GetEvent()
	total := len(ev.GetTitle()) + len(ev.GetBody()) + len(ev.GetLink()) + len(ev.GetResourceId()) + len(ev.GetId())
	if total > h.m.cfg.MaxEventBytes {
		t.Fatalf("event is %d bytes, limit %d", total, h.m.cfg.MaxEventBytes)
	}
	if _, err := h.m.Send(context.Background(), "example", strings.Repeat("s", h.m.cfg.MaxSecretBytes+1), Notification{}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized secret = %v, want ErrTooLarge", err)
	}
	h.rpc.sendResp.Detail = strings.Repeat("d", 10<<10)
	d, err := h.m.Send(context.Background(), "example", "", Notification{})
	if err != nil || len(d.Detail) > h.m.cfg.MaxResponseBytes {
		t.Fatalf("detail length = %d, err %v; want at most %d", len(d.Detail), err, h.m.cfg.MaxResponseBytes)
	}
}

func TestPluginEchoOfSecretIsRedacted(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	h.rpc.sendResp.Detail = "failed for token tok-123"
	d, err := h.m.Send(context.Background(), "example", "tok-123", Notification{})
	if err != nil || strings.Contains(d.Detail, "tok-123") {
		t.Fatalf("detail = %q, err %v", d.Detail, err)
	}
	h.rpc.sendErr = errors.New("rpc says tok-123 is bad")
	_, err = h.m.Send(context.Background(), "example", "tok-123", Notification{})
	if err == nil || strings.Contains(err.Error(), "tok-123") {
		t.Fatalf("error = %v, want the secret redacted", err)
	}
}

func TestAuditFailureRefusesTheCall(t *testing.T) {
	h := runningHarness(t, []string{AbilityRead})
	h.audit.fail = errBoom
	if _, err := h.m.Send(context.Background(), "example", "", Notification{}); err == nil {
		t.Fatal("call went through without an audit entry")
	}
	if len(h.rpc.sent) != 0 {
		t.Fatal("the plugin was called although the audit write failed")
	}
}

func TestConcurrencyCapAndCircuitBreaker(t *testing.T) {
	ctx := context.Background()
	h := runningHarness(t, []string{AbilityRead})
	h.rpc.block = make(chan struct{})
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := h.m.Send(ctx, "example", "", Notification{}); done <- err }()
	}
	deadline := time.Now().Add(2 * time.Second)
	in := h.m.inst("example")
	for len(in.sem) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := h.m.Send(ctx, "example", "", Notification{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("third concurrent call = %v, want ErrBusy", err)
	}
	close(h.rpc.block)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	h.rpc.block = nil
	h.rpc.sendErr = errBoom
	for range 2 {
		_, _ = h.m.Send(ctx, "example", "", Notification{})
	}
	if _, err := h.m.Send(ctx, "example", "", Notification{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("after repeated failures = %v, want ErrCircuitOpen", err)
	}
	h.advance(time.Minute)
	h.rpc.sendErr = nil
	if _, err := h.m.Send(ctx, "example", "", Notification{}); err != nil {
		t.Fatalf("after the cooldown = %v", err)
	}
}
