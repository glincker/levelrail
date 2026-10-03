// TestLoadBalancer_* closes docs/feature-status.md's e2e gap for the Load
// balancer feature: a real 2-replica app balanced by a real ingress
// Controller and embedded Caddy, real traffic spread across both real
// containers, a real admin-state drain, and the experimental gate itself,
// all over real HTTP. Out of scope: multi-node upstreams, the
// weighted/cookie/ip_hash algorithms, and probe classification under a
// real failing backend (internal/loadbalancer/status_test.go covers that
// against fakes).
package e2e

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eLBAdminUsername = "e2e-lb-admin"
	e2eLBAdminPassword = "e2e-lb-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// helloLBBodyPrefix is test/fixtures/hello-e2e-lb's distinctive response
// prefix; the fixture appends its own container hostname after it, which
// is what lets this test tell which real container answered a given real
// request.
const helloLBBodyPrefix = "hello from levelrail e2e lb replica "

// TestLoadBalancer_Live_RealTrafficAndDrain is this package's live proof
// for the LoadBalancer experimental feature. Skips cleanly if Docker or
// BuildKit aren't reachable, matching this package's other live tests.
func TestLoadBalancer_Live_RealTrafficAndDrain(t *testing.T) {
	defer experimental.Reset()
	experimental.Set(experimental.LoadBalancer)

	env := newLiveBuildEnv(t)
	dockerCli, buildClient, runtime := env.DockerCli, env.BuildClient, env.Runtime

	const (
		serviceName = "levelrail-test-e2e-lb"
		domain      = "e2e-lb.levelrail.internal"
	)
	repo := "levelrail/test-e2e-lb"
	tag := repo + ":e2elb1"

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = dockerCli.ImageRemove(cleanupCtx, tag, image.RemoveOptions{Force: true})
	})
	cleanupContainers(context.Background(), t, runtime, serviceName)
	t.Cleanup(func() { cleanupContainers(context.Background(), t, runtime, serviceName) })

	svcStore := openLiveStore(t)

	buildCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := buildClient.Build(buildCtx, build.Request{ContextDir: "../fixtures/hello-e2e-lb", Tag: tag}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Step 1: two real replicas, converged by a real application
	// Controller exactly the way a real "replicas: 2" app.yaml would be.
	desired := store.DesiredService{
		Name:     serviceName,
		Image:    res.Tag,
		Port:     8080,
		Domains:  []string{domain},
		Replicas: 2,
		Health:   &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/"}},
	}
	if err := svcStore.SaveDesiredService(buildCtx, desired); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	appCtrl := application.New(serviceName, svcStore, runtime)
	appResult, err := appCtrl.Reconcile(buildCtx)
	if err != nil {
		t.Fatalf("application Controller.Reconcile() error = %v, result = %+v", err, appResult)
	}
	if len(appResult.Conditions) == 0 || appResult.Conditions[0].Status != reconcile.ConditionTrue {
		t.Fatalf("application Controller.Reconcile() result = %+v, want a True Ready condition", appResult)
	}
	persistReadyCondition(buildCtx, t, svcStore, appCtrl.Name(), appResult.Conditions)

	// Independent verification: inspect both replica containers directly
	// through docker.Runtime for their real published dial addresses, and
	// through the raw Docker client for their real container hostnames
	// (test/fixtures/hello-e2e-lb bakes its own hostname into its response
	// body at container start), the ground truth this test checks real
	// HTTP response bodies against below, rather than trusting the
	// reconciler's say-so.
	dial := make([]string, 2)
	hostname := make([]string, 2)
	for i := 0; i < 2; i++ {
		name := application.ReplicaContainerName(serviceName, res.Tag, "", i)
		state, err := runtime.InspectByName(buildCtx, name)
		if err != nil {
			t.Fatalf("InspectByName(%q) error = %v", name, err)
		}
		if state == nil || !state.Running || len(state.Ports) == 0 {
			t.Fatalf("InspectByName(%q) = %+v, want a running container with a published port", name, state)
		}
		dial[i] = net.JoinHostPort("127.0.0.1", strconv.Itoa(state.Ports[0].HostPort))

		inspect, err := dockerCli.ContainerInspect(buildCtx, state.ID)
		if err != nil {
			t.Fatalf("raw ContainerInspect(%q) error = %v", state.ID, err)
		}
		if inspect.Config == nil || inspect.Config.Hostname == "" {
			t.Fatalf("ContainerInspect(%q) has no hostname: %+v", state.ID, inspect.Config)
		}
		hostname[i] = inspect.Config.Hostname
	}
	if dial[0] == dial[1] {
		t.Fatalf("both replicas published the same dial address %q, the test can't distinguish them", dial[0])
	}
	if hostname[0] == hostname[1] {
		t.Fatalf("both replicas reported the same hostname %q, the test can't distinguish them", hostname[0])
	}

	// Step 2: a real *api.Router and a real ingress Controller share one
	// loadbalancer.Registry, the same wiring a real control plane uses: the
	// API's PUT calls below and the ingress reconciler's own discovery
	// both act on the one real store and registry.
	registry := loadbalancer.NewRegistry()
	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	router := api.NewRouter(logger, b, svcStore, api.WithLoadBalancers(svcStore, registry, nil))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(buildCtx, svcStore, e2eLBAdminUsername, e2eLBAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eLBAdminUsername, e2eLBAdminPassword)

	// Step 3: configure the load balancer through the real HTTP API, not by
	// writing to the store directly: this is the operator-facing path
	// (dashboard, CLI) the feature exists to expose.
	status, body := requestJSON(t, client, http.MethodPut, ts.URL+"/api/v1/apps/"+serviceName+"/loadbalancer", `{"algorithm":"round_robin"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT loadbalancer: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	// Step 4: a real ingress Controller, on ephemeral ports, discovers both
	// real containers above and configures a real embedded Caddy to
	// balance across them.
	caddyAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	adminAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	driver := ingress.New(nil)
	t.Cleanup(func() {
		if err := driver.Stop(context.Background()); err != nil {
			t.Errorf("Driver.Stop() error = %v", err)
		}
	})
	ingressCtrl := ingressreconcile.New(svcStore, runtime, driver,
		ingressreconcile.WithServerName("e2e-lb"),
		ingressreconcile.WithListenAddr(caddyAddr),
		ingressreconcile.WithAdminListen(adminAddr),
		ingressreconcile.WithStorageDir(t.TempDir()),
		ingressreconcile.WithLoadBalancers(svcStore, registry),
	)

	ingressCtx, ingressCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer ingressCancel()
	ingressResult, err := ingressCtrl.Reconcile(ingressCtx)
	if err != nil {
		t.Fatalf("ingress Controller.Reconcile() error = %v, result = %+v", err, ingressResult)
	}
	if len(ingressResult.Conditions) == 0 || ingressResult.Conditions[0].Status != reconcile.ConditionTrue || ingressResult.Conditions[0].Reason != "Routed1Services" {
		t.Fatalf("ingress Controller.Reconcile() result = %+v, want a True/Routed1Services Ready condition", ingressResult)
	}
	assertLBCondition(t, ingressResult, reconcile.ConditionTrue, "Balancing")

	httpsClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, caddyAddr)
			},
			// Caddy's internal issuer root is never installed into this
			// process's trust store, matching deploy_test.go's identical
			// comment: only routing and load spreading are proven here.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // deliberate, see deploy_test.go's identical comment
		},
	}

	// Warm-up: Caddy's listeners come up asynchronously, so make sure it is
	// actually serving before the real spread assertion below.
	warmUp := getBodyWithRetry(t, httpsClient, "https://"+domain+"/")
	if !strings.Contains(warmUp, helloLBBodyPrefix) {
		t.Fatalf("warm-up response = %q, want it to contain %q", warmUp, helloLBBodyPrefix)
	}

	// Step 5: real HTTP traffic through the real embedded Caddy, counted
	// by which real container's own hostname shows up in each real
	// response body: the proof that round-robin genuinely spread traffic
	// across both real containers, not just that the reconciler built a
	// 2-upstream pool (controller_loadbalancer_test.go already proves
	// that against a fake runtime). Caddy's own admin-API request counter
	// is a live in-flight gauge, not a cumulative total, so it cannot
	// prove this after the fact; the response body can.
	const spreadRequests = 20
	counts := countResponsesByHostname(t, httpsClient, "https://"+domain+"/", spreadRequests, hostname)
	// Not required to be an even split: the one earlier warm-up request
	// already shifted round-robin's phase by one, and Caddy's own
	// selection isn't guaranteed to be a strict deterministic cycle. The
	// bar here is "both real containers genuinely took a meaningful share
	// of the traffic", which a lopsided split (e.g. 8/12 of 20) still
	// clearly satisfies; a near-zero count for either would not.
	const minShare = spreadRequests / 4
	for _, h := range hostname {
		if counts[h] < minShare {
			t.Errorf("container hostname %q answered %d of %d requests, want at least %d (real round-robin spread across both replicas): counts=%v", h, counts[h], spreadRequests, minShare, counts)
		}
	}

	// Step 6: drain upstream 0 through the real HTTP admin-state API, not
	// by writing to the store directly.
	upstreamID := loadbalancer.UpstreamID(serviceName, 0)
	status, body = requestJSON(t, client, http.MethodPut,
		ts.URL+"/api/v1/apps/"+serviceName+"/loadbalancer/upstreams/"+url.PathEscape(upstreamID),
		`{"admin_state":"draining"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT upstream admin state: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var upstreamStatus loadbalancer.UpstreamStatus
	if err := json.Unmarshal(body, &upstreamStatus); err != nil {
		t.Fatalf("decode upstream admin state response %s: %v", body, err)
	}
	if upstreamStatus.AdminState != loadbalancer.AdminDraining {
		t.Errorf("upstream admin state = %q, want %q", upstreamStatus.AdminState, loadbalancer.AdminDraining)
	}

	// Step 7: reconcile again so the real ingress Controller picks up the
	// drained state from the real store and rebuilds Caddy's upstream pool
	// without replica 0.
	ingressResult, err = ingressCtrl.Reconcile(ingressCtx)
	if err != nil {
		t.Fatalf("ingress Controller.Reconcile() (post-drain) error = %v, result = %+v", err, ingressResult)
	}
	assertLBCondition(t, ingressResult, reconcile.ConditionFalse, "UpstreamsDegraded")

	// Step 8: real traffic again, counted the same way: the drained
	// replica's hostname must never appear again, and every request must
	// land on the one still-active replica.
	const postDrainRequests = 10
	postDrainCounts := countResponsesByHostname(t, httpsClient, "https://"+domain+"/", postDrainRequests, hostname)
	if got := postDrainCounts[hostname[0]]; got != 0 {
		t.Errorf("drained container %q answered %d of %d post-drain requests, want 0", hostname[0], got, postDrainRequests)
	}
	if got := postDrainCounts[hostname[1]]; got != postDrainRequests {
		t.Errorf("remaining container %q answered %d of %d post-drain requests, want all %d routed to it", hostname[1], got, postDrainRequests, postDrainRequests)
	}
}

// countResponsesByHostname issues n real GET requests against url and
// tallies each real response by which of hostnames appears in its body,
// failing the test outright if a response contains none of them or more
// than one (the pool must have exactly one real container answering any
// given request).
func countResponsesByHostname(t *testing.T, client *http.Client, url string, n int, hostnames []string) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(hostnames))
	for i := 0; i < n; i++ {
		body := getBodyWithRetry(t, client, url)
		matched := ""
		for _, h := range hostnames {
			if strings.Contains(body, h) {
				if matched != "" {
					t.Fatalf("request %d response = %q, matches both %q and %q, want exactly one", i, body, matched, h)
				}
				matched = h
			}
		}
		if matched == "" {
			t.Fatalf("request %d response = %q, want it to contain one of %v", i, body, hostnames)
		}
		counts[matched]++
	}
	return counts
}

// TestLoadBalancer_GateOff_Returns404 proves, over a real *api.Router, a
// real HTTP server and a real authenticated session, that the load
// balancer routes answer 404 with the experimental-disabled error shape
// while the LoadBalancer feature is off, and that the real handlers (not a
// gate stub) run once it is turned on. internal/api/experimental_gate_test.go
// already proves this against experimentalGateMiddleware directly; this
// closes the gap of proving it through the real wired router end to end.
// Needs no Docker or BuildKit, unlike this file's other live test.
func TestLoadBalancer_GateOff_Returns404(t *testing.T) {
	defer experimental.Reset()

	svcStore := openLiveStore(t)
	logger := discardTestLogger()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	registry := loadbalancer.NewRegistry()
	router := api.NewRouter(logger, b, svcStore, api.WithLoadBalancers(svcStore, registry, nil))
	ts := newE2ETestServer(t, router)

	const (
		username = "e2e-lb-gate-admin"
		password = "e2e-lb-gate-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	)
	if err := api.BootstrapAdmin(context.Background(), svcStore, username, password); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, username, password)

	const gateAppName = "levelrail-test-e2e-lb-gate"
	if err := svcStore.SaveDesiredService(context.Background(), store.DesiredService{Name: gateAppName, Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	statusPath := "/api/v1/apps/" + gateAppName + "/loadbalancer/status"

	// Off: both routes answer 404 with the gate's own error shape, before
	// any handler-specific logic (app lookup, config lookup) ever runs.
	experimental.Set()
	for _, p := range []string{"/api/v1/loadbalancers", statusPath} {
		status, body := requestJSON(t, client, http.MethodGet, ts.URL+p, "")
		if status != http.StatusNotFound {
			t.Fatalf("%s off: status = %d, want %d, body = %s", p, status, http.StatusNotFound, body)
		}
		var gateErr struct {
			Code    string `json:"code"`
			Feature string `json:"feature"`
		}
		if err := json.Unmarshal(body, &gateErr); err != nil {
			t.Fatalf("%s off: decode body %s: %v", p, body, err)
		}
		if gateErr.Code != api.ExperimentalDisabledCode || gateErr.Feature != string(experimental.LoadBalancer) {
			t.Errorf("%s off: body = %+v, want code %q feature %q", p, gateErr, api.ExperimentalDisabledCode, experimental.LoadBalancer)
		}
	}

	// On: the real handlers run. The list route always answers 200; the
	// per-app status route legitimately 404s for a reason that has
	// nothing to do with the gate (no load balancer configured yet for
	// gateAppName), so this configures one first and expects a real 200
	// Pending response, proving the real handler ran rather than the gate
	// stub.
	experimental.Set(experimental.LoadBalancer)

	status, body := requestJSON(t, client, http.MethodGet, ts.URL+"/api/v1/loadbalancers", "")
	if status != http.StatusOK {
		t.Fatalf("list on: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	status, body = requestJSON(t, client, http.MethodPut, ts.URL+"/api/v1/apps/"+gateAppName+"/loadbalancer", `{"algorithm":"round_robin"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT loadbalancer on: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	status, body = requestJSON(t, client, http.MethodGet, ts.URL+statusPath, "")
	if status != http.StatusOK {
		t.Fatalf("status on: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var st loadbalancer.Status
	if err := json.Unmarshal(body, &st); err != nil {
		t.Fatalf("status on: decode body %s: %v", body, err)
	}
	if st.Reason != "Pending" {
		t.Errorf("status on: Reason = %q, want %q (no reconcile pass has observed this balancer yet)", st.Reason, "Pending")
	}
}

// assertLBCondition fails the test unless result carries a "LoadBalancer"
// condition matching wantStatus and wantReason exactly.
func assertLBCondition(t *testing.T, result reconcile.Result, wantStatus reconcile.ConditionStatus, wantReason string) {
	t.Helper()
	for _, c := range result.Conditions {
		if c.Type == "LoadBalancer" {
			if c.Status != wantStatus || c.Reason != wantReason {
				t.Errorf("LoadBalancer condition = %+v, want status %q reason %q", c, wantStatus, wantReason)
			}
			return
		}
	}
	t.Fatalf("no LoadBalancer condition in %+v", result.Conditions)
}
