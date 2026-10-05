package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/sshprovision"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeSSHProvisioner is a hand-written fake for SSHProvisioner: runs
// synchronously (no real network dial), reports whatever events/result a
// test configures, and records the creds/params it was called with so a
// test can assert credentials never leak past this one call.
type fakeSSHProvisioner struct {
	events []sshprovision.Event
	host   sshprovision.DetectedHost
	err    error

	lastCreds  sshprovision.Credentials
	lastParams sshprovision.InstallParams
}

func (f *fakeSSHProvisioner) Provision(_ context.Context, creds sshprovision.Credentials, params sshprovision.InstallParams, onEvent func(sshprovision.Event)) (sshprovision.DetectedHost, error) {
	f.lastCreds = creds
	f.lastParams = params
	for _, e := range f.events {
		if onEvent != nil {
			onEvent(e)
		}
	}
	return f.host, f.err
}

func newTestRouterWithSSHProvisioning(t *testing.T, fake *fakeSSHProvisioner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithSSHProvisioner(fake)), db
}

func createSSHProvisionBody() string {
	return `{
		"host": "192.0.2.10",
		"port": 2222,
		"username": "root",
		"auth": {"type": "password", "password": "s3cret"},
		"name": "ssh-web-1",
		"role": "general",
		"control_plane_addr": "cp.example:9443"
	}`
}

func awaitSSHProvisionStatus(t *testing.T, db *store.DB, id, want string) store.SSHNodeProvision {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		p, err := db.GetSSHNodeProvision(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if p.Status == want {
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("ssh node provision %s status = %q, want %q", id, p.Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHandleCreateSSHNodeProvision(t *testing.T) {
	fake := &fakeSSHProvisioner{
		events: []sshprovision.Event{
			{Step: sshprovision.StepConnect, Message: "connected"},
			{Step: sshprovision.StepDone, Message: "agent active"},
		},
		host: sshprovision.DetectedHost{OS: "linux", Arch: "amd64", Distro: "Ubuntu 24.04", SystemdPresent: true, DockerPresent: true},
	}
	rt, db := newTestRouterWithSSHProvisioning(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created sshNodeProvisionResource
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Name != "ssh-web-1" || created.Status != store.SSHNodeProvisionStatusConnecting {
		t.Fatalf("created = %+v", created)
	}
	// Never echo a credential back, even indirectly: nothing in the
	// response body's raw JSON should contain the password.
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatalf("response leaked the password: %s", rec.Body.String())
	}

	final := awaitSSHProvisionStatus(t, db, created.ID, store.SSHNodeProvisionStatusEnrolling)
	if final.DetectedOS != "linux" || final.DetectedArch != "amd64" {
		t.Fatalf("final = %+v", final)
	}
	if fake.lastCreds.Host != "192.0.2.10" || fake.lastCreds.Port != 2222 || fake.lastCreds.Username != "root" {
		t.Fatalf("provisioner called with unexpected creds: %+v", fake.lastCreds)
	}
	if fake.lastCreds.Password != "s3cret" {
		t.Fatalf("provisioner should have received the password, got %+v", fake.lastCreds)
	}
	if fake.lastParams.JoinToken == "" || fake.lastParams.ControlPlaneAddr != "cp.example:9443" || fake.lastParams.NodeName != "ssh-web-1" {
		t.Fatalf("provisioner called with unexpected params: %+v", fake.lastParams)
	}
	if strings.Contains(final.Log, "s3cret") {
		t.Fatalf("stored log leaked the password: %q", final.Log)
	}
}

func TestHandleCreateSSHNodeProvisionValidation(t *testing.T) {
	rt, db := newTestRouterWithSSHProvisioning(t, &fakeSSHProvisioner{})
	cookie := loginTestSession(t, rt, db)

	cases := []struct {
		name string
		body string
	}{
		{"missing host", `{"username":"root","auth":{"type":"password","password":"x"},"name":"a","control_plane_addr":"cp:9443"}`},
		{"missing username", `{"host":"1.2.3.4","auth":{"type":"password","password":"x"},"name":"a","control_plane_addr":"cp:9443"}`},
		{"missing control plane addr", `{"host":"1.2.3.4","username":"root","auth":{"type":"password","password":"x"},"name":"a"}`},
		{"bad name", `{"host":"1.2.3.4","username":"root","auth":{"type":"password","password":"x"},"name":"BadName!","control_plane_addr":"cp:9443"}`},
		{"newline in control plane addr", `{"host":"1.2.3.4","username":"root","auth":{"type":"password","password":"x"},"name":"a","control_plane_addr":"cp:9443\nAPP_X=1"}`},
		{"bad role", `{"host":"1.2.3.4","username":"root","auth":{"type":"password","password":"x"},"name":"a","role":"weird","control_plane_addr":"cp:9443"}`},
		{"bad auth type", `{"host":"1.2.3.4","username":"root","auth":{"type":"fingerprint"},"name":"a","control_plane_addr":"cp:9443"}`},
		{"key auth missing key", `{"host":"1.2.3.4","username":"root","auth":{"type":"key"},"name":"a","control_plane_addr":"cp:9443"}`},
		{"password auth missing password", `{"host":"1.2.3.4","username":"root","auth":{"type":"password"},"name":"a","control_plane_addr":"cp:9443"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleCreateSSHNodeProvisionNameCollision(t *testing.T) {
	rt, db := newTestRouterWithSSHProvisioning(t, &fakeSSHProvisioner{
		events: []sshprovision.Event{{Step: sshprovision.StepDone, Message: "done"}},
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409, body = %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleCreateSSHNodeProvisionFailure(t *testing.T) {
	fake := &fakeSSHProvisioner{
		events: []sshprovision.Event{
			{Step: sshprovision.StepDetect, Message: "unsupported OS", Err: context.DeadlineExceeded},
		},
		err: context.DeadlineExceeded,
	}
	rt, db := newTestRouterWithSSHProvisioning(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created sshNodeProvisionResource
	_ = json.NewDecoder(rec.Body).Decode(&created)

	final := awaitSSHProvisionStatus(t, db, created.ID, store.SSHNodeProvisionStatusFailed)
	if final.FailureReason == "" {
		t.Fatalf("expected a failure reason, got %+v", final)
	}
}

func TestHandleListAndGetSSHNodeProvisions(t *testing.T) {
	fake := &fakeSSHProvisioner{events: []sshprovision.Event{{Step: sshprovision.StepDone, Message: "done"}}}
	rt, db := newTestRouterWithSSHProvisioning(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	var created sshNodeProvisionResource
	_ = json.NewDecoder(rec.Body).Decode(&created)
	awaitSSHProvisionStatus(t, db, created.ID, store.SSHNodeProvisionStatusEnrolling)

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ssh-node-provisions", ""))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var list []sshNodeProvisionResource
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ssh-node-provisions/"+created.ID, ""))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	notFoundRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(notFoundRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ssh-node-provisions/missing", ""))
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d, want 404", notFoundRec.Code)
	}
}

func TestHandleGetSSHNodeProvisionEnrollmentCompletes(t *testing.T) {
	fake := &fakeSSHProvisioner{events: []sshprovision.Event{{Step: sshprovision.StepDone, Message: "done"}}}
	rt, db := newTestRouterWithSSHProvisioning(t, fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/ssh-provision", createSSHProvisionBody()))
	var created sshNodeProvisionResource
	_ = json.NewDecoder(rec.Body).Decode(&created)
	awaitSSHProvisionStatus(t, db, created.ID, store.SSHNodeProvisionStatusEnrolling)

	// Simulate the agent enrolling: a real node row now exists with the
	// same name the provision used.
	if err := db.SaveNode(context.Background(), store.Node{ID: "node_1", Name: "ssh-web-1", Status: store.NodeStatusOnline, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("SaveNode: %v", err)
	}

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ssh-node-provisions/"+created.ID, ""))
	var got sshNodeProvisionResource
	if err := json.NewDecoder(getRec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != store.SSHNodeProvisionStatusReady || got.NodeID != "node_1" {
		t.Fatalf("got = %+v", got)
	}

	n, err := db.GetNode(context.Background(), "node_1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if !n.AcceptsAppWorkloads {
		t.Fatalf("expected node to accept app workloads for a general-role provision, got %+v", n)
	}
}
