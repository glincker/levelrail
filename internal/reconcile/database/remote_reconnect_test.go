package database

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// unlabeledRuntime mimics an agent that does not report labels.
type unlabeledRuntime struct{ *fakeRuntime }

func (u unlabeledRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	st, err := u.fakeRuntime.InspectByName(ctx, name)
	if st != nil {
		st.Labels = nil
		st.LabelsUnknown = true
	}
	return st, err
}

// Unreported labels must not read as TLS, PITR or config drift: that would
// replace a data-bearing container on every reconnect.
func TestController_Reconcile_RemoteUnknownLabels_NeverRecreates(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("cache"), "redis:7", true)
	certPEM, keyPEM, err := GenerateSelfSignedCert("db-cache")
	if err != nil {
		t.Fatal(err)
	}
	desired := &store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis, Version: "7"}
	c := New("cache", &fakeStore{db: desired}, unlabeledRuntime{rt}, WithTLS(&TLSMaterial{CertPEM: certPEM, KeyPEM: keyPEM}))

	for i := 0; i < 3; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if cond := conditionOf(t, res); cond.Reason != "AlreadyRunning" {
			t.Fatalf("pass %d: reason = %q, want AlreadyRunning", i, cond.Reason)
		}
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 || rt.stopCalls != 0 {
		t.Errorf("create=%d remove=%d stop=%d, want none", rt.createCalls, rt.removeCalls, rt.stopCalls)
	}
}

// A node that drops off and comes back must produce errors while away and no
// create or remove at all once it answers again.
func TestController_Reconcile_RemoteOfflineAndBack_NoRecreate(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "postgres:16", true)
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}
	c := New("main", &fakeStore{db: desired}, podmanRuntime{rt}, WithPostgresCredentials(&PostgresCredentials{Username: "u", Password: "p"}))

	rt.inspectErr = errors.New("agent disconnected")
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("want an error while the node is unreachable")
	}
	rt.inspectErr = nil
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("after reconnect: %v", err)
	}
	if cond := conditionOf(t, res); cond.Reason != "AlreadyRunning" {
		t.Fatalf("reason = %q, want AlreadyRunning", cond.Reason)
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 || rt.stopCalls != 0 {
		t.Errorf("create=%d remove=%d stop=%d, want none", rt.createCalls, rt.removeCalls, rt.stopCalls)
	}
}

func TestController_Teardown_RemovesOwnCertsHelperOnly(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "postgres:16", false)
	rt.seed("dbcerts-aaa", "alpine:3.20", true)
	rt.seed("dbcerts-bbb", "alpine:3.20", true)
	rt.containers["dbcerts-aaa"].Labels = map[string]string{certsHelperLabelKey: certsVolumeName("main")}
	rt.containers["dbcerts-bbb"].Labels = map[string]string{certsHelperLabelKey: certsVolumeName("other")}
	c := New("main", &fakeStore{}, rt)

	if err := c.Teardown(context.Background()); err != nil {
		t.Fatalf("Teardown: %v", err)
	}
	if _, left := rt.containers["dbcerts-aaa"]; left {
		t.Error("this database's certs helper was left behind")
	}
	if _, kept := rt.containers["dbcerts-bbb"]; !kept {
		t.Error("another database's certs helper must not be touched")
	}
}
