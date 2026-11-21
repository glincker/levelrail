package alerting

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeployDispatcher_Dispatch_WebpushAudienceIsAppAndChannelProject(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Push", Kind: NotifyWebpush, Enabled: true, ProjectID: "proj_a"}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	if err := db.SaveDeployTarget(ctx, DeployTarget{ID: "dnt_1", ResourceID: "service:web", ChannelID: "chn_1", Enabled: true}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	sender := &fakePushSender{}
	NewDeployDispatcher(db, nil, nil, sender, nil).Dispatch(ctx, "service:web", DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})

	if sender.called != 1 {
		t.Fatalf("Send called %d times, want 1", sender.called)
	}
	if want := (PushAudience{AppName: "web", ProjectID: "proj_a"}); sender.audience != want {
		t.Errorf("audience = %+v, want %+v", sender.audience, want)
	}
}

func TestSendNotice_ProjectChannelNeedsAppAudience(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	c := NotificationChannel{ID: "chn_1", Name: "Push", Kind: NotifyWebpush, Enabled: true, ProjectID: "proj_a"}
	if err := db.SaveNotificationChannel(ctx, c); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	sender := &fakePushSender{}
	d := NewDeployDispatcher(db, nil, nil, sender, nil)

	if err := d.SendNotice(ctx, c, "instance wide"); !errors.Is(err, ErrNoticeSkipped) || sender.called != 0 {
		t.Fatalf("instance notice on project channel: err=%v called=%d, want ErrNoticeSkipped and no send", err, sender.called)
	}
	aud := PushAudience{AppName: "web", AdminOnly: true, ProjectID: "proj_a"}
	if err := d.SendNotice(WithPushAudience(ctx, aud), c, "app notice"); err != nil || sender.called != 1 {
		t.Fatalf("app notice: err=%v called=%d, want sent", err, sender.called)
	}
	if sender.audience != aud {
		t.Errorf("audience = %+v, want %+v", sender.audience, aud)
	}

	open := NotificationChannel{ID: "chn_2", Name: "Open", Kind: NotifyWebpush, Enabled: true}
	if err := db.SaveNotificationChannel(ctx, open); err != nil {
		t.Fatal(err)
	}
	if err := d.SendNotice(ctx, open, "instance wide"); err != nil || sender.called != 2 {
		t.Fatalf("unscoped channel: err=%v called=%d, want sent with the zero audience", err, sender.called)
	}
	if sender.audience != (PushAudience{}) {
		t.Errorf("instance notice audience = %+v, want the zero value", sender.audience)
	}
}

func TestEngineSendEvent_WebpushRuleAudience(t *testing.T) {
	db := newTestDeployNotifyDB(t)
	ctx := context.Background()
	if err := db.SaveNotificationChannel(ctx, NotificationChannel{ID: "chn_1", Name: "Push", Kind: NotifyWebpush, Enabled: true, ProjectID: "proj_a"}); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	sender := &fakePushSender{}
	engine := NewEngine(db, nil, nil, nil, nil, nil, 0, 0, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0,
		func(r Rule) Notifier { return NewNotifier(nil, nil, sender, r) }, nil)

	_ = engine.sendEvent(ctx, Event{Rule: Rule{ID: "rul_1", Name: "cpu", ResourceID: "service:web", ChannelID: "chn_1", NotifyKind: NotifyWebpush}})
	if want := (PushAudience{AppName: "web", ProjectID: "proj_a"}); sender.audience != want {
		t.Errorf("rule audience = %+v, want %+v", sender.audience, want)
	}
	_ = engine.sendEvent(ctx, Event{Rule: Rule{ID: "rul_2", Name: "node", ResourceID: "node:n1", ChannelID: "chn_1", NotifyKind: NotifyWebpush}})
	if sender.audience != (PushAudience{}) {
		t.Errorf("node rule audience = %+v, want the zero value", sender.audience)
	}
}

func TestPushAudienceForResource(t *testing.T) {
	if got := pushAudienceForResource("service:web"); got.AppName != "web" {
		t.Errorf("service resource audience = %+v", got)
	}
	for _, id := range []string{"node:n1", "database:db", "service:", ""} {
		if got := pushAudienceForResource(id); got != (PushAudience{}) {
			t.Errorf("%q audience = %+v, want the zero value", id, got)
		}
	}
}

type fakeDomainOwners struct {
	owner string
	found bool
}

func (f fakeDomainOwners) DomainOwnerOf(context.Context, string) (string, bool, error) {
	return f.owner, f.found, nil
}

func TestResolvePushAudience_DomainMapsToOwningAppAdmins(t *testing.T) {
	ctx := context.Background()
	got := resolvePushAudience(ctx, fakeDomainOwners{owner: "web", found: true}, "domain:shop.example.com")
	if want := (PushAudience{AppName: "web", AdminOnly: true}); got != want {
		t.Errorf("owned domain audience = %+v, want %+v", got, want)
	}
}

func TestResolvePushAudience_UnresolvedStaysAdminOnly(t *testing.T) {
	ctx := context.Background()
	cases := map[string]PushAudience{
		"unowned domain":     resolvePushAudience(ctx, fakeDomainOwners{}, "domain:free.example.com"),
		"parked domain":      resolvePushAudience(ctx, fakeDomainOwners{owner: store.ParkedDomainOwner, found: true}, "domain:p.example.com"),
		"no resolver":        resolvePushAudience(ctx, struct{}{}, "domain:x.example.com"),
		"node has no team":   resolvePushAudience(ctx, fakeDomainOwners{owner: "web", found: true}, "node:n1"),
		"volume has no team": resolvePushAudience(ctx, fakeDomainOwners{owner: "web", found: true}, "volume:v1"),
	}
	for name, got := range cases {
		if got != (PushAudience{}) {
			t.Errorf("%s audience = %+v, want the zero admin-only audience", name, got)
		}
	}
}
