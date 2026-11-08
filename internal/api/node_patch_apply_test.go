package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const aptSimFixture = `Reading package lists...
Inst libssl3 [3.0.2-0ubuntu1.10] (3.0.2-0ubuntu1.12 Ubuntu:22.04/jammy-updates, Ubuntu:22.04/jammy-security [amd64])
Inst linux-image-5.15.0-91-generic [5.15.0-89.99] (5.15.0-91.101 Ubuntu:22.04/jammy-security [amd64])
Inst docker-ce [5:24.0.6-1] (5:24.0.7-1 Docker CE:jammy/stable [amd64])
Inst containerd.io [1.6.24-1] (1.6.25-1 Ubuntu:22.04/jammy-security [amd64])
Inst vim [2:8.2.3995-1ubuntu2.12] (2:8.2.3995-1ubuntu2.13 Ubuntu:22.04/jammy-updates [amd64])
Conf libssl3 (3.0.2-0ubuntu1.12 Ubuntu:22.04/jammy-updates [amd64])
`

type fakePatchRunner struct {
	mu       sync.Mutex
	sim      string
	calls    [][]string
	reboot   bool
	systemd  bool
	block    chan struct{} // non-nil makes a direct install wait until closed
	hang     bool          // systemctl show blocks until its context ends
	extra    string        // extra lines the install simulation prints beyond the requested names
	units    string        // systemctl list-units output
	states   map[string]string
	listFail int  // list-units fails this many more times
	simFail  bool // the install simulation exits 100
}

const patchFakeOutput = "Setting up libssl3\nurl https://user:hunter2@mirror.example\npassword=hunter2\n" //nolint:gosec // fake value that proves masking, not a credential

func (f *fakePatchRunner) FileExists(path string) bool {
	return path == patchBinAptGet || (path == aptRebootMarkerPath && f.reboot)
}
func (f *fakePatchRunner) UnderSystemd() bool { return f.systemd }
func (f *fakePatchRunner) Run(ctx context.Context, _ []string, name string, args ...string) ([]byte, int, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	block, sim, hang, extra, units, simFail := f.block, f.sim, f.hang, f.extra, f.units, f.simFail
	first, state, hasState := "", "", false
	if len(args) > 0 {
		first = args[0]
		state, hasState = f.states[args[len(args)-1]]
	}
	listFailing := name == patchBinSystemctl && first == "list-units" && f.listFail > 0
	if listFailing {
		f.listFail--
	}
	f.mu.Unlock()
	switch {
	case listFailing:
		return []byte("Failed to connect to bus"), 1, nil
	case simFail && name == patchBinAptGet && first == "-s" && len(args) > 1 && args[1] == "install":
		return []byte("E: Unable to correct problems, you have held broken packages."), 100, nil
	case hang && name == patchBinSystemctl && first == "show":
		<-ctx.Done()
		return nil, -1, ctx.Err()
	case name == patchBinAptGet && first == "-s" && len(args) > 1 && args[1] == "install":
		return []byte(fakeInstallSim(args) + extra), 0, nil
	case name == patchBinAptGet && first == "-s":
		return []byte(sim), 0, nil
	case name == patchBinAptGet && block != nil:
		<-block
	case name == patchBinSystemctl && first == "show" && hasState:
		return []byte(state), 0, nil
	case name == patchBinSystemctl && first == "show":
		return []byte("LoadState=loaded\nActiveState=active\nSubState=exited\nResult=success\nExecMainStatus=0\n"), 0, nil
	case name == patchBinSystemctl && first == "list-units":
		return []byte(units), 0, nil
	case name == patchBinSystemctl, name == patchBinSystemdRun:
		return nil, 0, nil
	}
	return []byte(patchFakeOutput), 0, nil
}

// fakeInstallSim prints one Inst line per pinned name=version spec, as apt-get -s install would.
func fakeInstallSim(args []string) string {
	out := ""
	for _, a := range args {
		if n, v, ok := strings.Cut(a, "="); ok && !strings.HasPrefix(a, "-") && !strings.Contains(n, "::") {
			out += "Inst " + n + " [0] (" + v + " Ubuntu:22.04/jammy-security [amd64])\n"
		}
	}
	return out
}

func (f *fakePatchRunner) callContaining(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

func (f *fakePatchRunner) setState(unit, state string) {
	f.mu.Lock()
	f.states[unit] = state
	f.mu.Unlock()
}

func (f *fakePatchRunner) call(bin, first string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c[0] == bin && (first == "" || (len(c) > 1 && c[1] == first)) {
			return c
		}
	}
	return nil
}

func (f *fakePatchRunner) setSim(s string) {
	f.mu.Lock()
	f.sim = s
	f.mu.Unlock()
}

func (f *fakePatchRunner) installCall() []string { return f.call(patchBinAptGet, "install") }

func patchTestRouter(t *testing.T) (*Router, *http.Cookie, *fakePatchRunner) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake := &fakePatchRunner{sim: aptSimFixture}
	rt.nodePatch = newNodePatcher(fake)
	rt.nodePatch.done = make(chan string, 4)
	return rt, cookie, fake
}

func patchDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func patchPreview(t *testing.T, rt *Router, cookie *http.Cookie, body string) nodePatchPreviewResponse {
	t.Helper()
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/preview", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var p nodePatchPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	return p
}

func patchApplyBody(previewID string, includeAll, reboot bool) string {
	b, _ := json.Marshal(nodePatchApplyRequest{PreviewID: previewID, IncludeAll: includeAll, RebootIfRequired: reboot, Confirm: true})
	return string(b)
}

func TestParseAptSimulation(t *testing.T) {
	pkgs := parseAptSimulation(aptSimFixture)
	if len(pkgs) != 5 {
		t.Fatalf("parsed %d packages, want 5: %+v", len(pkgs), pkgs)
	}
	byName := map[string]nodePatchPackage{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if p := byName["libssl3"]; !p.Security || p.CurrentVersion != "3.0.2-0ubuntu1.10" || p.NewVersion != "3.0.2-0ubuntu1.12" {
		t.Errorf("libssl3 = %+v", p)
	}
	if !byName["linux-image-5.15.0-91-generic"].Kernel || !byName["docker-ce"].ContainerRuntime || !byName["containerd.io"].ContainerRuntime {
		t.Errorf("kernel or runtime flags missing: %+v", pkgs)
	}
	if byName["vim"].Security {
		t.Errorf("vim marked security")
	}
}

func TestNodePatchPreviewHoldsKernelAndRuntimeWithoutIncludeAll(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	p := patchPreview(t, rt, cookie, `{}`)
	if len(p.Packages) != 1 || p.Packages[0].Name != "libssl3" {
		t.Fatalf("packages = %+v, want only libssl3", p.Packages)
	}
	if len(p.Held) != 2 || p.OtherAvailable != 2 || p.PreviewID == "" {
		t.Fatalf("held = %+v other = %d id = %q", p.Held, p.OtherAvailable, p.PreviewID)
	}
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	<-rt.nodePatch.done
	call := strings.Join(fake.installCall(), " ")
	if !strings.Contains(call, "libssl3=3.0.2-0ubuntu1.12") || !strings.Contains(call, "--no-remove") || strings.Contains(call, "linux-image") || strings.Contains(call, "docker") || strings.Contains(call, "containerd") {
		t.Fatalf("install argv = %q", call)
	}

	all := patchPreview(t, rt, cookie, `{"include_all":true}`)
	if len(all.Packages) != 5 || len(all.Held) != 0 || !all.RebootLikely {
		t.Fatalf("include_all preview = %+v", all)
	}
}

func TestNodePatchApplyRejectsStalePreview(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, true, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "preview_mismatch") {
		t.Fatalf("include_all mismatch status = %d, body = %s", rec.Code, rec.Body.String())
	}
	fake.setSim(aptSimFixture + "Inst openssl [3.0.2-0ubuntu1.10] (3.0.2-0ubuntu1.12 Ubuntu:22.04/jammy-security [amd64])\n")
	rec = patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "preview_stale") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody("bogus", false, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "preview_expired") {
		t.Fatalf("unknown preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.installCall() != nil {
		t.Fatal("install ran on a stale preview")
	}
}

func TestNodePatchApplyRejectedDuringDeploy(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	p := patchPreview(t, rt, cookie, `{}`)
	rt.buildStartMu.Lock()
	rt.startingDeploys = map[string]int{"web": 1}
	rt.buildStartMu.Unlock()
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "deploy_in_progress") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.installCall() != nil {
		t.Fatal("install ran during a deploy")
	}
}

func TestNodePatchRebootRefusedWhenAppsWouldGoDown(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake := &fakePatchRunner{sim: aptSimFixture}
	rt.nodePatch = newNodePatcher(fake)
	seedApp(t, db, "web")
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, true))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "drain_required") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestNodePatchApplyOneAtATimeAndAudited(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.nodePatch = newNodePatcher(&fakePatchRunner{sim: aptSimFixture})
	rt.nodePatch.done = make(chan string, 4)
	p := patchPreview(t, rt, cookie, `{}`)
	rt.nodePatch.mu.Lock()
	rt.nodePatch.busy[NodeGuardrailLocalID] = true
	rt.nodePatch.mu.Unlock()
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "patch_in_progress") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rt.nodePatch.release(NodeGuardrailLocalID)
	rec = patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	<-rt.nodePatch.done
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := false
	for _, e := range entries {
		found = found || (strings.Contains(e.Path, "/patch/apply#phase=finish") && strings.Contains(e.Path, "status=succeeded") && strings.Contains(e.Path, "packages=libssl3") && strings.Contains(e.Path, "held=linux-image"))
	}
	if !found {
		t.Fatalf("no patch apply audit entry in %d entries", len(entries))
	}
}

func TestNodePatchRunHistoryMasksOutput(t *testing.T) {
	rt, cookie, _ := patchTestRouter(t)
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	select {
	case <-rt.nodePatch.done:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not finish")
	}
	rec = patchDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/local/patch/runs", "")
	var runs []nodePatchRun
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 1 {
		t.Fatalf("runs = %s (%v)", rec.Body.String(), err)
	}
	r := runs[0]
	if r.Status != patchRunSucceeded || r.FinishedAt == nil || r.PatchStatusAfter == nil || len(r.Held) != 2 {
		t.Fatalf("run = %+v", r)
	}
	if strings.Contains(r.OutputTail, "hunter2") {
		t.Fatalf("output tail leaks a secret: %q", r.OutputTail)
	}
}

func TestNodePatchRuntimeUpgradeRefusedWithoutSystemd(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	p := patchPreview(t, rt, cookie, `{"include_all":true}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, true, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "runtime_upgrade_needs_systemd") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.installCall() != nil {
		t.Fatal("install ran without systemd")
	}
}

func TestNodePatchDetachedUnitArgvIsFixed(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	fake.systemd = true
	rt.nodePatch.pollEvery = time.Millisecond
	p := patchPreview(t, rt, cookie, `{"include_all":true}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, true, false))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	<-rt.nodePatch.done
	if fake.installCall() != nil {
		t.Fatal("install ran inside the control plane process instead of a unit")
	}
	argv := fake.call(patchBinSystemdRun, "")
	joined := strings.Join(argv, " ")
	if len(argv) < 2 || !strings.HasPrefix(argv[1], "--unit=") || !strings.Contains(argv[1], "-patch-") ||
		!strings.Contains(joined, "--setenv=NEEDRESTART_MODE=l") || !strings.Contains(joined, "-- "+patchBinAptGet+" install --only-upgrade --no-remove") ||
		!strings.Contains(joined, "docker-ce=5:24.0.7-1") || !strings.Contains(joined, "--description=all=true pkgs=libssl3,") {
		t.Fatalf("systemd-run argv = %q", joined)
	}
	for _, a := range argv {
		if a == "sh" || a == "-c" || strings.Contains(a, "/bin/sh") {
			t.Fatalf("shell in argv %q", joined)
		}
	}
	rec = patchDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/local/patch/runs", "")
	var runs []nodePatchRun
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 1 || runs[0].Status != patchRunSucceeded || strings.Contains(runs[0].OutputTail, "hunter2") {
		t.Fatalf("runs = %s (%v)", rec.Body.String(), err)
	}
}

func TestNodePatchTimeoutStopsWaitingWithoutKilling(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	fake.block = make(chan struct{})
	rt.nodePatch.timeout = 20 * time.Millisecond
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	<-rt.nodePatch.done
	rec = patchDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/local/patch/runs", "")
	var runs []nodePatchRun
	if err := json.Unmarshal(rec.Body.Bytes(), &runs); err != nil || len(runs) != 1 || runs[0].Status != patchRunTimedOut {
		t.Fatalf("runs = %s (%v)", rec.Body.String(), err)
	}
	if !patchLocalBusy(rt) {
		t.Fatal("node not busy while the timed out install still runs")
	}
	close(fake.block)
	<-rt.nodePatch.done
	if r := adoptedRuns(rt)[runs[0].ID]; patchLocalBusy(rt) || r.Status != patchRunSucceeded {
		t.Fatalf("late result not settled: %+v", r)
	}
}

func TestPatchChildEnvIsMinimal(t *testing.T) {
	t.Setenv("APP_MASTER_KEY", "should-not-leak")
	t.Setenv("LD_PRELOAD", "/tmp/x.so")
	allowed := map[string]bool{"PATH": true, "LANG": true, "LC_ALL": true, "DEBIAN_FRONTEND": true, "APT_LISTCHANGES_FRONTEND": true, "NEEDRESTART_MODE": true, "NEEDRESTART_SUSPEND": true}
	for _, kv := range patchChildEnv() {
		k, _, _ := strings.Cut(kv, "=")
		if !allowed[k] {
			t.Errorf("unexpected env key %q", k)
		}
	}
	env := strings.Join(patchChildEnv(), "\n")
	if !strings.Contains(env, "NEEDRESTART_MODE=l") || !strings.Contains(env, "NEEDRESTART_SUSPEND=1") || strings.Contains(env, "should-not-leak") {
		t.Fatalf("env = %q", env)
	}
}

func TestParsePatchUnitShowNeverTreatsFailureAsSuccess(t *testing.T) {
	cases := []struct {
		name, out string
		done      bool
		code      int
	}{
		{"success", "ActiveState=active\nSubState=exited\nResult=success\nExecMainStatus=0", true, 0},
		{"failed with zero", "ActiveState=failed\nSubState=failed\nResult=exit-code\nExecMainStatus=0", true, -1},
		{"failed empty status", "ActiveState=failed\nResult=signal\nExecMainStatus=", true, -1},
		{"exited not success", "ActiveState=active\nSubState=exited\nResult=timeout\nExecMainStatus=0", true, -1},
		{"failed with code", "ActiveState=failed\nResult=exit-code\nExecMainStatus=100", true, 100},
		{"running", "ActiveState=active\nSubState=running\nResult=success", false, 0},
	}
	for _, c := range cases {
		st := parsePatchUnitShow(c.out)
		if st.done != c.done || st.exitCode != c.code {
			t.Errorf("%s: got %+v, want done=%t code=%d", c.name, st, c.done, c.code)
		}
	}
}

func TestNodePatchHungUnitQueryReleasesLock(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	fake.systemd, fake.hang = true, true
	rt.nodePatch.pollEvery = time.Millisecond
	rt.nodePatch.timeout = 50 * time.Millisecond
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, true))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	select {
	case <-rt.nodePatch.done:
	case <-time.After(5 * time.Second):
		t.Fatal("run never finished while systemctl hung")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		rt.nodePatch.mu.Lock()
		busy := rt.nodePatch.busy[NodeGuardrailLocalID]
		rt.nodePatch.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("node lock leaked after a hung unit query")
		}
		time.Sleep(5 * time.Millisecond)
	}
	rt.nodePatch.mu.Lock()
	runs := append([]nodePatchRun{}, rt.nodePatch.runs[NodeGuardrailLocalID]...)
	rt.nodePatch.mu.Unlock()
	if len(runs) != 1 || runs[0].Status != patchRunTimedOut || runs[0].RebootScheduled {
		t.Fatalf("runs = %+v", runs)
	}
	if fake.call(patchBinShutdown, "") != nil {
		t.Fatal("rebooted after a run that did not succeed")
	}
	if !patchLocalBusy(rt) || fake.callContaining("stop --quiet") {
		t.Fatal("the lock left with the request, but the still running unit must keep the node busy and unstopped")
	}
	fake.mu.Lock()
	fake.hang = false
	fake.mu.Unlock()
	select {
	case <-rt.nodePatch.done:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher never settled the unit")
	}
	if patchLocalBusy(rt) || !fake.callContaining("stop --quiet") || fake.call(patchBinShutdown, "") != nil {
		t.Fatal("finished unit not released, not cleaned up, or rebooted late")
	}
}
