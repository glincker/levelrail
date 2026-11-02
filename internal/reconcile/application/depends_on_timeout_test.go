package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDependsOnTimeoutFailsNamingTheDependency(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}
	now := time.Unix(1000, 0)
	c := New("myapp-web", &fakeStore{svc: desired}, rt,
		WithDependsOnTimeout(func(context.Context) time.Duration { return 2 * time.Minute }))
	c.now = func() time.Time { return now }

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionUnknown || cond.Reason != "WaitingForDependency" {
		t.Fatalf("inside the limit: %+v", cond)
	}

	now = now.Add(3 * time.Minute)
	res, _ = c.Reconcile(context.Background())
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != reasonDependencyTimeout || !strings.Contains(cond.Message, `"db"`) {
		t.Fatalf("after the limit: %+v", cond)
	}
	if rt.createCalls != 0 {
		t.Fatalf("createCalls = %d, a timed out dependent must not start", rt.createCalls)
	}

	// The dependency coming up later still lets the service through and resets the clock.
	rt.seed(replicaContainerName("myapp-db", "postgres:16", "", 0), true)
	res, _ = c.Reconcile(context.Background())
	if cond := conditionOf(t, res); cond.Reason == reasonDependencyTimeout {
		t.Fatalf("late dependency must clear the timeout: %+v", cond)
	}
	if _, ok := c.dependsOnSince["myapp-web"]; ok {
		t.Fatal("wait start not cleared")
	}
}

func TestDependsOnWithoutTimeoutOptionWaitsForever(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "myapp-web", AppID: "myapp", Image: "img:v1", Port: 80, DependsOn: []string{"db"}}
	c := New("myapp-web", &fakeStore{svc: desired}, rt)
	start := time.Unix(0, 0)
	c.now = func() time.Time { return start.Add(48 * time.Hour) }
	res, _ := c.Reconcile(context.Background())
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionUnknown {
		t.Fatalf("no option keeps the old wait: %+v", cond)
	}
}
