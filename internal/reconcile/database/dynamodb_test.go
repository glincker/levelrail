package database

import (
	"context"
	"slices"
	"testing"

	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/store"
)

func TestController_Reconcile_DynamoDB_Spec(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "kv", Engine: store.EngineDynamoDB, Version: "3.3.2"}
	c := New("kv", &fakeStore{db: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition = %+v, want True", cond)
	}
	if got := ImageRef(store.EngineDynamoDB, "3.3.2"); got != "amazon/dynamodb-local:3.3.2" {
		t.Errorf("image = %q", got)
	}
	cmd := dynamoDBCommand(nil)
	if !slices.Contains(cmd, "-sharedDb") || !slices.Contains(cmd, dynamoDBDataPath) {
		t.Errorf("command = %v, want -sharedDb and the data path", cmd)
	}
}

func TestController_Reconcile_DynamoDB_RefusesPublicBind(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "kv", Engine: store.EngineDynamoDB, Version: "3.3.2", PubliclyAccessible: true, PublicPort: 40011, PublicBindAddress: "0.0.0.0"}
	c := New("kv", &fakeStore{db: desired}, rt)
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "UnauthenticatedExposureRefused" {
		t.Errorf("reason = %q", cond.Reason)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0", rt.createCalls)
	}
	if !RefusesPublicBind(store.EngineDynamoDB, false, "0.0.0.0") {
		t.Error("guard must refuse an all interfaces bind")
	}
	if RefusesPublicBind(store.EngineDynamoDB, false, "127.0.0.1") {
		t.Error("loopback must be allowed")
	}
}

func TestDynamoDBVersionRules(t *testing.T) {
	for _, bad := range []string{"latest", "1.25.1", "3.3"} {
		if err := ValidateEngineVersion(store.EngineDynamoDB, bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if err := CheckInPlaceVersionChange(store.EngineDynamoDB, "3.3.1", "3.3.2"); err != nil {
		t.Errorf("same major refused: %v", err)
	}
	if err := CheckInPlaceVersionChange(store.EngineDynamoDB, "2.6.1", "3.3.2"); err == nil {
		t.Error("major change must be refused")
	}
	if got := ConventionalEnvVar(store.EngineDynamoDB, "url"); got != "DYNAMODB_ENDPOINT" {
		t.Errorf("env var = %q", got)
	}
}
