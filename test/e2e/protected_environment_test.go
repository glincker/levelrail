// TestProtectedEnvironment_Live_DeployBlockedThenAllowed is this
// package's live proof for the deploy-approval gate internal/api's
// TestHandleTriggerDeploy_ProtectedEnvironment (deploys_test.go) and
// TestHandleApproveDeployApproval/TestHandleApproveDeployApproval_SameActorForbidden
// (deploy_approvals_test.go) already cover at the handler level: those
// tests assert status codes and store rows using httptest.NewRecorder
// against handlers directly. What they cannot show is the thing that
// actually matters operationally: whether the gate genuinely stops a
// container from changing until a real, distinct second user approves
// it, or only stops a JSON field from changing while some other path
// still mutates the running deployment. This test drives the whole
// two-person approval gate (deploy_approvals.go) over a real TCP
// connection, two real cookie-authenticated sessions (a requester and a
// separate, distinct approver), and a real application.Controller
// reconciling real Docker containers: an unconfirmed request, a
// confirmed-but-not-yet-approved request, and a same-actor approval
// attempt all leave the running container untouched, and only a
// different, sufficiently privileged user's approval actually cuts it
// over.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eProtectedEnvAdminUsername = "e2e-protected-env-admin"
	e2eProtectedEnvAdminPassword = "e2e-protected-env-correct-horse" //nolint:gosec // test fixture credential, not a real secret

	// e2eProtectedEnvApproverEmail/Password identify a genuinely
	// distinct second actor: a real, separately-created user account
	// holding only AbilityDeploy (not root), proving the approval gate
	// works for the least-privileged account that can actually approve
	// anything, not just for a root admin approving itself under a
	// different label.
	e2eProtectedEnvApproverEmail    = "e2e-protected-env-approver@example.com"
	e2eProtectedEnvApproverPassword = "e2e-protected-env-approver-horse" //nolint:gosec // test fixture credential, not a real secret

	// e2eProtectedEnvServiceName is this test's single app name,
	// hoisted to package scope (not a local const inside the test func)
	// so assertRunningWithImage below can reference it directly instead
	// of taking it as a parameter every one of its four call sites in
	// this file would otherwise pass the identical value for.
	e2eProtectedEnvServiceName = "levelrail-test-e2e-protected-env"
)

func TestProtectedEnvironment_Live_DeployBlockedThenAllowed(t *testing.T) {
	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	const serviceName = e2eProtectedEnvServiceName
	repo := "levelrail/test-e2e-protected-env"
	tagA := repo + ":e2eprotectedenva"
	tagB := repo + ":e2eprotectedenvb"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagA, image.RemoveOptions{Force: true})
		_, _ = dockerCli.ImageRemove(cleanupCtx, tagB, image.RemoveOptions{Force: true})
	})

	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Two real builds of the identical fixture content under distinct
	// tags, the same "container identity is keyed by image tag, not
	// content" reasoning rollback_test.go's own package comment
	// documents: what's under test is whether the confirmation gate lets
	// an image change reach Docker, not what the served body is.
	resA, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tagA}, nil)
	if err != nil {
		t.Fatalf("Build(A) error = %v", err)
	}
	resB, err := buildClient.Build(ctx, build.Request{ContextDir: "../fixtures/hello-e2e", Tag: tagB}, nil)
	if err != nil {
		t.Fatalf("Build(B) error = %v", err)
	}

	// Step 1: a real project, a real protected environment, and a real
	// app already tagged into it and already running image A, matching
	// the state an operator's real production environment would be in
	// before anyone tries to deploy again.
	if err := svcStore.SaveProject(ctx, store.Project{ID: "proj_e2e_protected", Name: "e2e-protected", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := svcStore.SaveEnvironment(ctx, store.Environment{ID: "env_e2e_protected_prod", ProjectID: "proj_e2e_protected", Name: "production", Protected: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
	if err := svcStore.SaveDesiredService(ctx, store.DesiredService{
		Name:  serviceName,
		Image: resA.Tag,
		Port:  8080,
		Health: &store.ServiceHealth{
			Readiness: &store.ServiceProbe{Path: "/"},
		},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := svcStore.SetServiceEnvironment(ctx, serviceName, "env_e2e_protected_prod"); err != nil {
		t.Fatalf("SetServiceEnvironment() error = %v", err)
	}

	appCtrl := application.New(serviceName, svcStore, runtime)
	nameA := application.ContainerName(serviceName, resA.Tag, "")
	nameB := application.ContainerName(serviceName, resB.Tag, "")

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("initial Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, nameA, resA.Tag)

	// Step 2: a real *api.Router, a real HTTP server, and a real
	// cookie-authenticated session, the same construction
	// test/e2e/metrics_test.go and auth_lifecycle_test.go already use.
	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore)
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, svcStore, e2eProtectedEnvAdminUsername, e2eProtectedEnvAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eProtectedEnvAdminUsername, e2eProtectedEnvAdminPassword)

	// Step 3: the unconfirmed deploy. The real HTTP response must be
	// 409, and, more importantly, the running container must remain
	// exactly image A after a second real reconcile: this is the actual
	// safety property, not the status code alone.
	status, body := postJSON(t, client, ts.URL+"/api/v1/apps/"+serviceName+"/deploys", `{"image":"`+resB.Tag+`"}`)
	if status != http.StatusConflict {
		t.Fatalf("unconfirmed deploy: status = %d, want %d, body = %s", status, http.StatusConflict, body)
	}
	if !strings.Contains(body, "protected") {
		t.Errorf("unconfirmed deploy: body = %s, want it to mention the environment is protected", body)
	}

	svc, err := svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resA.Tag {
		t.Fatalf("desired state Image = %q after a rejected unconfirmed deploy, want unchanged %q", svc.Image, resA.Tag)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-block Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, nameA, resA.Tag)
	assertContainerAbsent(ctx, t, runtime, nameB, "B")

	// Step 4: the confirmed deploy. A real 202, but the deploy-approval
	// gate (deploy_approvals.go) means this does not apply yet: the
	// response carries pending_approval, not app, and desired state (and
	// the running container) must stay on image A until a distinct
	// second user approves it.
	status, body = postJSON(t, client, ts.URL+"/api/v1/apps/"+serviceName+"/deploys", `{"image":"`+resB.Tag+`","confirm":true}`)
	if status != http.StatusAccepted {
		t.Fatalf("confirmed deploy: status = %d, want %d, body = %s", status, http.StatusAccepted, body)
	}
	approvalID := decodePendingApprovalID(t, body)

	svc, err = svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resA.Tag {
		t.Fatalf("desired state Image = %q after a confirmed deploy accepted as a pending approval, want it to stay unchanged at %q until approved", svc.Image, resA.Tag)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-confirm-pending Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, nameA, resA.Tag)
	assertContainerAbsent(ctx, t, runtime, nameB, "B")

	// Step 5: the same actor that requested the deploy tries to approve
	// its own request. A real 403, and the running container still
	// unchanged.
	status, body = postJSON(t, client, ts.URL+"/api/v1/deploy-approvals/"+approvalID+"/approve", "")
	if status != http.StatusForbidden {
		t.Fatalf("self-approve: status = %d, want %d, body = %s", status, http.StatusForbidden, body)
	}

	svc, err = svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resA.Tag {
		t.Fatalf("desired state Image = %q after a rejected self-approval, want unchanged %q", svc.Image, resA.Tag)
	}

	// Step 6: a genuinely distinct second user, holding only
	// AbilityDeploy, approves it. A real 200, a real store update, and a
	// real reconcile that genuinely swaps the running container to
	// image B.
	approver := createE2EUser(t, svcStore, e2eProtectedEnvApproverEmail, e2eProtectedEnvApproverPassword, []string{api.AbilityDeploy})
	approverClient := loginE2EClient(t, ts.URL, approver.Email, e2eProtectedEnvApproverPassword)

	status, body = postJSON(t, approverClient, ts.URL+"/api/v1/deploy-approvals/"+approvalID+"/approve", "")
	if status != http.StatusOK {
		t.Fatalf("approve: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	svc, err = svcStore.GetDesiredService(ctx, serviceName)
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != resB.Tag {
		t.Fatalf("desired state Image = %q after approval, want %q", svc.Image, resB.Tag)
	}

	if result, err := appCtrl.Reconcile(ctx); err != nil {
		t.Fatalf("post-approve Reconcile() error = %v, result = %+v", err, result)
	}
	assertRunningWithImage(ctx, t, runtime, nameB, resB.Tag)
	assertContainerAbsent(ctx, t, runtime, nameA, "A")
}

// decodePendingApprovalID extracts pending_approval.id from a
// deployTriggerResult JSON body (internal/api/deploys.go): the approval
// ID this test needs to drive the approve/reject routes next, failing
// loudly if the confirmed deploy came back applied instead of pending
// (the exact regression this test guards against).
func decodePendingApprovalID(t *testing.T, body string) string {
	t.Helper()
	var result struct {
		PendingApproval *struct {
			ID string `json:"id"`
		} `json:"pending_approval"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("decode confirmed-deploy response: %v; body = %s", err, body)
	}
	if result.PendingApproval == nil || result.PendingApproval.ID == "" {
		t.Fatalf("confirmed deploy into a protected environment did not come back as a pending approval; body = %s", body)
	}
	return result.PendingApproval.ID
}

// createE2EUser inserts a real, individually-identified user account
// directly via the store (the same shape api.BootstrapAdmin uses to hash
// a password), for a genuinely distinct second actor this package's live
// tests need to log in as over a real HTTP session, not the bootstrap
// admin under a different label.
func createE2EUser(t *testing.T, svcStore *store.DB, email, password string, abilities []string) store.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword() error = %v", err)
	}
	hashStr := string(hash)
	u := store.User{
		ID:           "user_" + email,
		Email:        email,
		DisplayName:  email,
		PasswordHash: &hashStr,
		Abilities:    abilities,
		CreatedAt:    time.Now().UTC(),
	}
	if err := svcStore.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser(%q) error = %v", email, err)
	}
	return u
}

// assertRunningWithImage verifies, directly against docker.Runtime, that
// exactly one container exists for e2eProtectedEnvServiceName, is
// running, and matches both wantName and wantImage: the same
// independent-of-Reconcile's-own-return-value rigor this package's other
// live tests apply throughout. Only ever called with that one service
// name in this file, so it's not a parameter.
func assertRunningWithImage(ctx context.Context, t *testing.T, runtime docker.Runtime, wantName, wantImage string) {
	t.Helper()
	containers, err := runtime.ListByPrefix(ctx, e2eProtectedEnvServiceName+"-")
	if err != nil {
		t.Fatalf("ListByPrefix() error = %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected exactly 1 container for %q, got %d: %+v", e2eProtectedEnvServiceName, len(containers), containers)
	}
	if !containers[0].Running {
		t.Fatalf("container %+v is not running", containers[0])
	}
	if containers[0].Name != wantName {
		t.Fatalf("running container's name = %q, want %q", containers[0].Name, wantName)
	}
	if containers[0].Image != wantImage {
		t.Fatalf("running container's image = %q, want %q", containers[0].Image, wantImage)
	}
}

// loginE2EClient logs in against a real POST /api/v1/auth/login route
// through a fresh cookie jar, the same pattern
// test/e2e/metrics_test.go's TestMetrics_Live_ContainerToHTTP already
// uses, factored out here since two more live tests in this package
// (compose healthcheck and master key rotation) need the identical
// login flow.
func loginE2EClient(t *testing.T, baseURL, username, password string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: e2eHTTPTimeout}

	status, body := postJSON(t, client, baseURL+"/api/v1/auth/login", `{"username":"`+username+`","password":"`+password+`"}`)
	if status != http.StatusOK {
		t.Fatalf("login: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	return client
}
