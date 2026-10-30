package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// prefixLister mimics Docker's name filter: a plain prefix match.
type prefixLister struct{ all []docker.ContainerState }

func (p prefixLister) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	var out []docker.ContainerState
	for _, c := range p.all {
		if len(c.Name) >= len(prefix) && c.Name[:len(prefix)] == prefix {
			out = append(out, c)
		}
	}
	return out, nil
}

type fixedSecrets struct{ ComposeSecretStore }

func (fixedSecrets) Resolve(context.Context, string, string) (string, error) {
	return "fake-admin-value-for-tests", nil
}

func mailIdentityRouter(t *testing.T) (*Router, store.DesiredService, string) {
	t.Helper()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	svc := store.DesiredService{Name: "mail-mox", Image: "r.xmox.nl/mox:v0.0.17", Port: 8080, TemplateID: mailTemplateID}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	saved, err := db.GetDesiredService(ctx, svc.Name)
	if err != nil {
		t.Fatal(err)
	}
	info, err := rt.instanceInfo(ctx)
	if err != nil || info.InstanceID == "" {
		t.Fatalf("instance id: %v", err)
	}
	rt.composeSecrets = fixedSecrets{}
	return rt, *saved, info.InstanceID
}

// TestMailEngineIgnoresPrefixSpoof models the exploit: a tenant app whose
// name starts with the mail app's name sets levelrail.mail=engine and listens
// on the admin port. It must never be chosen and never see the admin login.
func TestMailEngineIgnoresPrefixSpoof(t *testing.T) {
	var hits atomic.Int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer attacker.Close()
	_, port, _ := net.SplitHostPort(attacker.Listener.Addr().String())
	t.Setenv(mox.AdminPortEnv, port)

	rt, svc, instanceID := mailIdentityRouter(t)
	engineName := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	rt.containers = prefixLister{all: []docker.ContainerState{
		// The engineName engine is stopped, so nothing legitimate is reachable.
		{Name: engineName, Running: false, Labels: map[string]string{appspec.InstanceLabelKey: instanceID}},
		{Name: "mail-moxx-0123456789ab", Running: true,
			Labels:   map[string]string{"levelrail.mail": "engine", appspec.InstanceLabelKey: instanceID},
			Networks: []docker.ContainerNetwork{{Name: "app", IPAddress: "127.0.0.1"}}},
		{Name: engineName + "x", Running: true,
			Labels:   map[string]string{"levelrail.mail": "engine", appspec.InstanceLabelKey: instanceID},
			Networks: []docker.ContainerNetwork{{Name: "app", IPAddress: "127.0.0.1"}}},
	}}
	e, err := rt.defaultMailEngine(context.Background(), svc.Name)
	if err == nil {
		_, _ = e.Domains(context.Background())
		t.Fatal("a stopped engine must fail closed, not fall through to a look-alike container")
	}
	if st := rt.mailProbe(context.Background(), svc); st.ContainerState != "stopped" || st.AdminReachable {
		t.Fatalf("status reported the spoof: %+v", st)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the spoofed container received %d requests", n)
	}
}

func TestMailEngineContainerIdentity(t *testing.T) {
	rt, svc, instanceID := mailIdentityRouter(t)
	engineName := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	own := docker.ContainerState{Name: engineName, Running: true, Labels: map[string]string{appspec.InstanceLabelKey: instanceID},
		Networks: []docker.ContainerNetwork{{IPAddress: "10.0.0.5"}}}
	rt.containers = prefixLister{all: []docker.ContainerState{own}}
	if c, err := rt.mailEngineContainer(context.Background(), svc); err != nil || containerIP(c) != "10.0.0.5" {
		t.Fatalf("own engine: %v %+v", err, c)
	}
	foreign := own
	foreign.Labels = map[string]string{appspec.InstanceLabelKey: "other-instance"}
	rt.containers = prefixLister{all: []docker.ContainerState{foreign}}
	if _, err := rt.mailEngineContainer(context.Background(), svc); !errors.Is(err, errMailEngineForeign) {
		t.Fatalf("foreign instance: %v", err)
	}
	rt.containers = prefixLister{all: []docker.ContainerState{own, own}}
	if _, err := rt.mailEngineContainer(context.Background(), svc); !errors.Is(err, errMailEngineAmbiguous) {
		t.Fatalf("two matches must fail closed: %v", err)
	}
	notMail := svc
	notMail.TemplateID = ""
	if _, err := rt.mailEngineContainer(context.Background(), notMail); !errors.Is(err, errMailEngineNotMail) {
		t.Fatalf("non mail service: %v", err)
	}
}

type pendingBoxes struct {
	fakeBoxes
	pending map[string]bool
}

func (p *pendingBoxes) SetMailboxSuspendPending(_ context.Context, address string) error {
	p.pending[address] = true
	return nil
}

func (p *pendingBoxes) SetMailboxSuspended(ctx context.Context, address string, s bool) error {
	delete(p.pending, address)
	return p.fakeBoxes.SetMailboxSuspended(ctx, address, s)
}

func (p *pendingBoxes) ListMailboxesSuspendPending(context.Context) ([]store.Mailbox, error) {
	var out []store.Mailbox
	for a := range p.pending {
		out = append(out, p.boxes[a])
	}
	return out, nil
}

func TestOffboardingEngineDownIsPersistedAndRetried(t *testing.T) {
	rt, db := newTestRouter(t)
	boxes := &pendingBoxes{fakeBoxes: fakeBoxes{boxes: map[string]store.Mailbox{
		"jo@example.test": {Address: "jo@example.test", Service: "mail-mox", TeamID: "team-a", LinkedUserID: "u1"},
	}}, pending: map[string]bool{}}
	rt.mail.boxes = boxes
	rt.mail.engine = func(context.Context, string) (mail.Engine, error) { return nil, errMailEngineMissing }
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/teams/team-a/members/u1", nil)
	if n := rt.offboardMailboxes(req, "team-a", "u1"); n != 0 || !boxes.pending["jo@example.test"] {
		t.Fatalf("engine down must leave a pending record: n=%d pending=%v", n, boxes.pending)
	}
	entries, err := db.ListAuditEntries(context.Background(), 10, nil, store.AuditEntryFilter{})
	if err != nil || len(entries) == 0 || entries[0].Path != "/api/v1/mail/mailboxes/jo@example.test#suspend-pending" {
		t.Fatalf("pending suspend must be audited: %v %+v", err, entries)
	}
	eng := &fakeMailEngine{disabled: map[string]string{}}
	rt.retryPendingSuspends(req, eng, "mail-mox")
	if boxes.pending["jo@example.test"] || !boxes.boxes["jo@example.test"].Suspended || eng.disabled["jo@example.test"] == "" {
		t.Fatal("the next engine call must apply the owed suspend")
	}
}

// TestMailDomainListUsesMailDecide pins the domain list to the same rule as
// mailboxes: a Deny on the domain hides it.
func TestMailDomainListUsesMailDecide(t *testing.T) {
	deny := []store.Policy{{Document: `{"Statement":[{"Effect":"Deny","Action":["read"],"Resource":["mail:domain:hidden.test"]}]}`}}
	if got := mailDecide(nil, []string{AbilityRoot}, deny, "team-a", AbilityRead, mailResDomain+"hidden.test", mailResService+"mail-mox"); got != http.StatusNotFound {
		t.Fatalf("denied domain decision %d", got)
	}
	if got := mailDecide(nil, []string{AbilityRoot}, deny, "team-a", AbilityRead, mailResDomain+"shown.test", mailResService+"mail-mox"); got != 0 {
		t.Fatalf("allowed domain decision %d", got)
	}
}

func TestLinkedUserMustBeTeamMember(t *testing.T) {
	rt, _ := newTestRouter(t)
	if rt.isTeamMember(context.Background(), "team-a", "not-a-member") {
		t.Fatal("a user outside the team must not be linkable")
	}
	rt.policies = nil
	rt.apps = nil
	if rt.isTeamMember(context.Background(), "team-a", "u1") {
		t.Fatal("no membership store must fail closed")
	}
}
