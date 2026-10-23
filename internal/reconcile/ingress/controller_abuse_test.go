package ingress

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/abuse"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type abuseFakeStore struct {
	*fakeStore
	rows  []store.AppAbusePolicy
	err   error
	conds map[string]string
}

func (a *abuseFakeStore) ListAppAbusePolicies(context.Context) ([]store.AppAbusePolicy, error) {
	return a.rows, a.err
}

func (a *abuseFakeStore) SetAppAbuseCondition(_ context.Context, app, _, reason, _ string) error {
	if a.conds == nil {
		a.conds = map[string]string{}
	}
	a.conds[app] = reason
	return nil
}

func abuseFixture(rows []store.AppAbusePolicy, settings store.IngressSettings) (*Controller, *fakeApplier, *abuseFakeStore) {
	web := store.DesiredService{Name: "abuse-web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}}
	api := store.DesiredService{Name: "abuse-api", Image: "api:v1", Port: 80, Domains: []string{"api.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 34001)
	rt.seedRunning(application.ContainerName(api.Name, api.Image, ""), 34002)
	st := &abuseFakeStore{fakeStore: &fakeStore{services: []store.DesiredService{web, api}, settings: settings}, rows: rows}
	applier := &fakeApplier{}
	return New(st, rt, applier, WithLogger(discardLogger())), applier, st
}

func TestController_AbusePoliciesAttachPerApp(t *testing.T) {
	rows := []store.AppAbusePolicy{
		{App: "abuse-web", Spec: []byte(`{"mode":"enforce","rps":5,"burst":10}`)},
		{App: "abuse-api", Spec: []byte(`{"mode":"off"}`)},
	}
	c, applier, st := abuseFixture(rows, store.IngressSettings{})
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := routeJSON(t, applier, "a.example.com"); !strings.Contains(got, `"handler":"levelrail_abuse"`) || !strings.Contains(got, `"app":"abuse-web"`) {
		t.Fatalf("route has no abuse handler: %s", got)
	}
	if got := routeJSON(t, applier, "api.example.com"); strings.Contains(got, "levelrail_abuse") {
		t.Fatalf("an app with the defense off got the handler: %s", got)
	}
	if st.conds["abuse-web"] != AbuseReasonEnforcing || st.conds["abuse-api"] != AbuseReasonOff {
		t.Fatalf("conditions = %v", st.conds)
	}
}

func TestController_AbuseLogOnlyConditionAndHiddenClientIP(t *testing.T) {
	rows := []store.AppAbusePolicy{{App: "abuse-web", Spec: []byte(`{"mode":"log_only","rps":5}`)}}
	c, _, st := abuseFixture(rows, store.IngressSettings{})
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.conds["abuse-web"] != AbuseReasonLogOnly {
		t.Fatalf("conditions = %v", st.conds)
	}
	c, _, st = abuseFixture(rows, store.IngressSettings{TLSTerminatedUpstream: true})
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st.conds["abuse-web"] != AbuseReasonClientIPHidden {
		t.Fatalf("a hidden client address went unreported: %v", st.conds)
	}
}

func TestController_AbuseStoreFailureFailsOpenAndIsCounted(t *testing.T) {
	rows := []store.AppAbusePolicy{{App: "abuse-web", Spec: []byte(`{"mode":"enforce","rps":5}`)}}
	c, applier, st := abuseFixture(rows, store.IngressSettings{})
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	abuse.DefaultRegistry().DrainAll()
	st.err = errors.New("database is locked")
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("a policy store failure failed the whole pass: %v", err)
	}
	if got := routeJSON(t, applier, "a.example.com"); strings.Contains(got, "levelrail_abuse") {
		t.Fatalf("route kept the defense while the store was down: %s", got)
	}
	if got := abuse.DefaultRegistry().DrainAll()["abuse-web"].FailOpen; got != 1 {
		t.Fatalf("FailOpen = %d, want 1", got)
	}
}

func TestController_AbuseInvalidSpecFailsOpenForThatAppOnly(t *testing.T) {
	rows := []store.AppAbusePolicy{
		{App: "abuse-web", Spec: []byte(`{not json`)},
		{App: "abuse-api", Spec: []byte(`{"mode":"enforce","deny_user_agents":["badbot"]}`)},
	}
	c, applier, st := abuseFixture(rows, store.IngressSettings{})
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(routeJSON(t, applier, "a.example.com"), "levelrail_abuse") {
		t.Fatal("undecodable policy was applied")
	}
	if !strings.Contains(routeJSON(t, applier, "api.example.com"), "levelrail_abuse") {
		t.Fatal("a bad row stopped other apps' policies")
	}
	if st.conds["abuse-web"] != AbuseReasonInvalidPolicy {
		t.Fatalf("conditions = %v", st.conds)
	}
}
