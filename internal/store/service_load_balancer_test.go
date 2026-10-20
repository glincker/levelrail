package store

import (
	"context"
	"errors"
	"testing"
)

func TestServiceLoadBalancer_RoundTripAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetServiceLoadBalancer(ctx, "ghost", `{}`); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Set on missing service error = %v, want ErrServiceNotFound", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.GetServiceLoadBalancer(ctx, "web"); err != nil || found {
		t.Fatalf("Get before set = found %v err %v, want not found", found, err)
	}
	if err := db.SetServiceLoadBalancer(ctx, "web", `{"algorithm":"least_conn"}`); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceLoadBalancer(ctx, "web", `{"algorithm":"ip_hash"}`); err != nil {
		t.Fatal(err)
	}
	got, found, err := db.GetServiceLoadBalancer(ctx, "web")
	if err != nil || !found || got != `{"algorithm":"ip_hash"}` {
		t.Fatalf("Get = %q found %v err %v, want the overwritten value", got, found, err)
	}
	all, err := db.ListServiceLoadBalancers(ctx)
	if err != nil || len(all) != 1 || all["web"] != got {
		t.Fatalf("List = %v err %v", all, err)
	}

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetServiceLoadBalancer(ctx, "web"); !found {
		t.Fatal("redeploy must not drop the load balancer config")
	}

	if err := db.DeleteServiceLoadBalancer(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetServiceLoadBalancer(ctx, "web"); found {
		t.Fatal("found after delete")
	}
}
