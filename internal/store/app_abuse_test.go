package store

import (
	"context"
	"testing"
)

func TestAppAbusePolicyRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.GetAppAbusePolicy(ctx, "web"); err != nil || found {
		t.Fatalf("missing policy: found=%v err=%v", found, err)
	}
	if err := db.SetAppAbusePolicy(ctx, "web", []byte(`{"mode":"log_only","rps":5}`), "admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppAbuseCondition(ctx, "web", "True", "LogOnly", "observing"); err != nil {
		t.Fatal(err)
	}
	p, found, err := db.GetAppAbusePolicy(ctx, "web")
	if err != nil || !found || string(p.Spec) != `{"mode":"log_only","rps":5}` || p.UpdatedBy != "admin" || p.ConditionReason != "LogOnly" || p.ConditionAt == "" {
		t.Fatalf("policy = %+v found=%v err=%v", p, found, err)
	}
	at := p.ConditionAt
	if err := db.SetAppAbuseCondition(ctx, "web", "True", "LogOnly", "observing"); err != nil {
		t.Fatal(err)
	}
	if p, _, _ = db.GetAppAbusePolicy(ctx, "web"); p.ConditionAt != at {
		t.Fatal("an unchanged condition rewrote the row")
	}
	if err := db.SetAppAbusePolicy(ctx, "web", []byte(`{"mode":"enforce","rps":5}`), "admin"); err != nil {
		t.Fatal(err)
	}
	if p, _, _ = db.GetAppAbusePolicy(ctx, "web"); p.ConditionReason != "" {
		t.Fatalf("saving a new spec kept the stale condition: %+v", p)
	}
	all, err := db.ListAppAbusePolicies(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list = %+v err=%v", all, err)
	}
	if err := db.DeleteAppAbusePolicy(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAppAbusePolicy(ctx, "web"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if _, found, _ = db.GetAppAbusePolicy(ctx, "web"); found {
		t.Fatal("policy survived delete")
	}
}

func TestAppAbusePolicyFollowsAppDeletion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "gone", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppAbusePolicy(ctx, "gone", []byte(`{}`), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM desired_services WHERE name = ?`, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetAppAbusePolicy(ctx, "gone"); found {
		t.Fatal("policy outlived its app")
	}
}
