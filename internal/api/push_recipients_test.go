package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	webpushgo "github.com/SherClockHolmes/webpush-go"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webpush"
)

func TestPushRecipients_AllowPush(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	if err := f.db.SaveDesiredService(ctx, store.DesiredService{Name: "plain-web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	root := storeUserWithAbilitiesForTest(t, f.db, "root@default.test", []string{"root"})
	viewer := storeUserWithAbilitiesForTest(t, f.db, "viewer@default.test", []string{"read"})
	dev := storeUserWithAbilitiesForTest(t, f.db, "dev@default.test", []string{"read", "write"})
	admin := storeUserWithAbilitiesForTest(t, f.db, "admin@default.test", []string{"read", "write", "deploy"})
	teamOnly := f.members[store.TeamRoleOwner]

	policy := NewPushRecipients(f.db)
	instance := alerting.PushAudience{}
	app := alerting.PushAudience{AppName: "plain-web"}
	appAdmin := alerting.PushAudience{AppName: "plain-web", AdminOnly: true}
	otherTeamApp := alerting.PushAudience{AppName: appB}

	cases := []struct {
		name string
		user string
		aud  alerting.PushAudience
		want bool
	}{
		{"root instance notice", root.ID, instance, true},
		{"admin instance notice", admin.ID, instance, true},
		{"developer instance notice", dev.ID, instance, false},
		{"viewer instance notice", viewer.ID, instance, false},
		{"viewer reads app event", viewer.ID, app, true},
		{"viewer admin only app event", viewer.ID, appAdmin, false},
		{"developer admin only app event", dev.ID, appAdmin, false},
		{"admin admin only app event", admin.ID, appAdmin, true},
		{"root other team app", root.ID, otherTeamApp, true},
		{"viewer other team app", viewer.ID, otherTeamApp, false},
		{"admin other team app", admin.ID, otherTeamApp, false},
		{"team only user", teamOnly.ID, alerting.PushAudience{AppName: appA}, false},
		{"channel project mismatch", admin.ID, alerting.PushAudience{AppName: "plain-web", ProjectID: projB}, false},
		{"root channel project mismatch", root.ID, alerting.PushAudience{AppName: appA, ProjectID: projB}, false},
		{"root channel project match", root.ID, alerting.PushAudience{AppName: appA, ProjectID: projA}, true},
		{"admin root only notice", admin.ID, alerting.PushAudience{RootOnly: true}, false},
		{"root root only notice", root.ID, alerting.PushAudience{RootOnly: true}, true},
		{"own digest", viewer.ID, alerting.PushAudience{UserID: viewer.ID}, true},
		{"someone else's digest", admin.ID, alerting.PushAudience{UserID: viewer.ID}, false},
		{"unknown user", "user_missing", instance, false},
	}
	for _, tc := range cases {
		if got := policy.AllowPush(ctx, tc.user, tc.aud); got != tc.want {
			t.Errorf("%s: AllowPush = %v, want %v", tc.name, got, tc.want)
		}
	}

	if err := f.db.RemoveTeamMember(ctx, store.DefaultTeamID, viewer.ID); err != nil {
		t.Fatal(err)
	}
	if policy.AllowPush(ctx, viewer.ID, app) {
		t.Error("removed default team member still allowed")
	}
}

// TestPushRecipients_ThroughSender wires the sender the way cmd/levelrail does.
func TestPushRecipients_ThroughSender(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	root := storeUserWithAbilitiesForTest(t, f.db, "root2@default.test", []string{"root"})
	viewer := storeUserWithAbilitiesForTest(t, f.db, "viewer2@default.test", []string{"read"})
	for _, u := range []store.User{root, viewer} {
		if err := f.db.SavePushSubscription(ctx, store.PushSubscription{ID: "push_" + u.ID, UserID: u.ID, Endpoint: srv.URL + "/" + u.ID,
			P256dh: pushTestP256dh, Auth: pushTestAuth, CreatedAt: time.Now().UTC()}, 0); err != nil {
			t.Fatal(err)
		}
	}
	priv, pub, err := webpushgo.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	sender := webpush.NewSender(f.db, pub, priv, "ops@example.com", srv.Client(), nil).
		WithPolicy(NewPushRecipients(f.db)).
		WithEndpointValidator(func(string) error { return nil })
	if err := sender.Send(ctx, "t", "b", alerting.PushAudience{RootOnly: true}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["/"+root.ID] != 1 || hits["/"+viewer.ID] != 0 {
		t.Errorf("hits = %v, want only the root account to receive", hits)
	}
}
