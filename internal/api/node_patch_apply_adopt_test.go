package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	unitSucceeded = "LoadState=loaded\nActiveState=active\nSubState=exited\nResult=success\nExecMainStatus=0\n"
	unitRunning   = "LoadState=loaded\nActiveState=active\nSubState=running\nResult=success\nExecMainStatus=0\n"
)

type patchAdoptEnv struct {
	rt     *Router
	cookie *http.Cookie
	fake   *fakePatchRunner
	prefix string
	audit  func() ([]store.AuditEntry, error)
}

func patchAdoptRouter(t *testing.T) patchAdoptEnv {
	t.Helper()
	rt, db := newTestRouter(t)
	fake := &fakePatchRunner{sim: aptSimFixture, systemd: true, states: map[string]string{}}
	rt.nodePatch = newNodePatcher(fake)
	rt.nodePatch.done = make(chan string, 4)
	rt.nodePatch.pollEvery = time.Millisecond
	return patchAdoptEnv{rt: rt, cookie: loginTestSession(t, rt, db), fake: fake, prefix: patchStem(rt.patchBrandShort()) + "-patch-",
		audit: func() ([]store.AuditEntry, error) {
			return db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
		}}
}

func patchLocalBusy(rt *Router) bool {
	rt.nodePatch.mu.Lock()
	defer rt.nodePatch.mu.Unlock()
	return rt.nodePatch.nodeBusyLocked(NodeGuardrailLocalID)
}

func waitPatchDone(t *testing.T, rt *Router, n int) {
	t.Helper()
	for range n {
		select {
		case <-rt.nodePatch.done:
		case <-time.After(5 * time.Second):
			t.Fatal("adopted unit never settled")
		}
	}
}

func adoptedRuns(rt *Router) map[string]nodePatchRun {
	rt.nodePatch.mu.Lock()
	defer rt.nodePatch.mu.Unlock()
	out := map[string]nodePatchRun{}
	for _, r := range rt.nodePatch.runs[NodeGuardrailLocalID] {
		out[r.ID] = r
	}
	return out
}

func TestNodePatchAdoptsFinishedUnits(t *testing.T) {
	env := patchAdoptRouter(t)
	rt, fake, prefix := env.rt, env.fake, env.prefix
	ok, bad := prefix+"0123456789abcdef.service", prefix+"fedcba9876543210.service"
	fake.units = ok + " loaded active exited all=false\n" + bad + " loaded failed failed x\nother.service loaded active running y\n" +
		prefix + "XYZ.service loaded active exited z\n" + prefix + "0123456789abcdef0.service loaded active exited z\n"
	fake.states[ok] = unitSucceeded + "Description=all=false pkgs=libssl3,openssl\n"
	fake.states[bad] = "LoadState=loaded\nActiveState=failed\nSubState=failed\nResult=exit-code\nExecMainStatus=0\nDescription=all=true pkgs=vim\n"
	rt.StartNodePatchAdoption(context.Background())
	waitPatchDone(t, rt, 2)

	runs := adoptedRuns(rt)
	if len(runs) != 2 {
		t.Fatalf("adopted %d runs, want 2: %+v", len(runs), runs)
	}
	if r := runs["0123456789abcdef"]; r.Status != patchRunSucceeded || strings.Join(r.Packages, " ") != "libssl3 openssl" || strings.Contains(r.OutputTail, "hunter2") {
		t.Fatalf("succeeded unit run = %+v", r)
	}
	if r := runs["fedcba9876543210"]; r.Status != patchRunFailed || r.ExitCode != -1 || !r.IncludeAll {
		t.Fatalf("failed unit with exit 0 = %+v", r)
	}
	if !fake.callContaining("stop --quiet -- "+ok) || !fake.callContaining("reset-failed --quiet -- "+bad) {
		t.Fatal("finished units were not cleaned up")
	}
	if fake.callContaining("other.service") || fake.callContaining("XYZ") || fake.callContaining("0123456789abcdef0") {
		t.Fatal("touched a unit whose name does not match")
	}
	if !fake.callContaining("list-units --all --plain --no-legend --type=service -- " + prefix + "*") {
		t.Fatal("list-units argv changed")
	}
	entries, err := env.audit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := false
	for _, e := range entries {
		found = found || (strings.Contains(e.Path, "phase=finish") && strings.Contains(e.Path, "packages=libssl3 openssl") && e.ActorType == "system")
	}
	if !found {
		t.Fatalf("no adoption audit entry in %d entries", len(entries))
	}
}

func TestNodePatchAdoptedRunningUnitHoldsLockAndIsNeverStopped(t *testing.T) {
	env := patchAdoptRouter(t)
	rt, fake, prefix, cookie := env.rt, env.fake, env.prefix, env.cookie
	unit := prefix + "00112233aabbccdd.service"
	fake.units = unit + " loaded active running x\n"
	fake.states[unit] = unitRunning + "Description=all=false pkgs=libssl3\n"
	rt.StartNodePatchAdoption(context.Background())
	<-rt.nodePatch.scanned
	if !patchLocalBusy(rt) {
		t.Fatal("node not busy while an adopted unit runs")
	}
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "patch_in_progress") {
		t.Fatalf("second apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	time.Sleep(20 * time.Millisecond)
	if fake.callContaining("stop --quiet -- "+unit) || fake.call(patchBinSystemdRun, "") != nil {
		t.Fatal("stopped a running unit or started a second one")
	}
	fake.setState(unit, unitSucceeded+"Description=all=false pkgs=libssl3\n")
	waitPatchDone(t, rt, 1)
	if r := adoptedRuns(rt)["00112233aabbccdd"]; r.Status != patchRunSucceeded || r.FinishedAt == nil {
		t.Fatalf("adopted run = %+v", r)
	}
	if patchLocalBusy(rt) || !fake.callContaining("stop --quiet -- "+unit) {
		t.Fatal("finished unit not released or not cleaned up")
	}
}

func TestNodePatchAdoptIsBounded(t *testing.T) {
	env := patchAdoptRouter(t)
	rt, fake, prefix := env.rt, env.fake, env.prefix
	rt.nodePatch.adoptMax = 2
	for _, id := range []string{"1111111111111111", "2222222222222222", "3333333333333333"} {
		fake.units += prefix + id + ".service loaded active exited x\n"
	}
	rt.StartNodePatchAdoption(context.Background())
	waitPatchDone(t, rt, 2)
	if n := len(adoptedRuns(rt)); n != 2 || fake.callContaining("3333333333333333") {
		t.Fatalf("adopted %d runs past the limit", n)
	}
}

func TestNodePatchApplyRefusesExtraDependencies(t *testing.T) {
	rt, cookie, fake := patchTestRouter(t)
	fake.extra = "Inst libcrypto9 (1.0 Ubuntu:22.04/jammy-security [amd64])\nRemv oldlib [1.0]\n"
	p := patchPreview(t, rt, cookie, `{}`)
	rec := patchDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/local/patch/apply", patchApplyBody(p.PreviewID, false, false))
	body := rec.Body.String()
	if rec.Code != http.StatusConflict || !strings.Contains(body, "preview_mismatch") || !strings.Contains(body, "libcrypto9") || !strings.Contains(body, "oldlib (remove)") {
		t.Fatalf("status = %d, body = %s", rec.Code, body)
	}
	if fake.installCall() != nil {
		t.Fatal("install ran although it would pull in other packages")
	}
}

func TestPatchRPMVersionsAndSimulation(t *testing.T) {
	pkgs := parseCheckUpdate("foo.x86_64  1.2^20230101-1.fc39  updates\nbad.x86_64  1;2  updates\n")
	if len(pkgs) != 1 || pkgs[0].NewVersion != "1.2^20230101-1.fc39" {
		t.Fatalf("parsed = %+v", pkgs)
	}
	argv := patchInstallArgv(patchManagerDNF, patchBinDNF, pkgs)
	if strings.Join(argv, " ") != patchBinDNF+" -y -q upgrade foo-1.2^20230101-1.fc39.x86_64" {
		t.Fatalf("argv = %q", argv)
	}
	if sim := strings.Join(patchInstallSimArgv(patchManagerDNF, argv), " "); sim != patchBinDNF+" --assumeno upgrade foo-1.2^20230101-1.fc39.x86_64" {
		t.Fatalf("sim argv = %q", sim)
	}
	out := "Dependencies resolved.\n Package  Arch  Version  Repository  Size\nUpgrading:\n foo  x86_64  1.2^20230101-1.fc39  updates  1.0 M\n" +
		"Installing dependencies:\n bar  x86_64  2.0-1  updates  10 k\nRemoving:\n baz  noarch  1-1  @System  1 k\nTransaction Summary\n"
	extras, ok := patchSimulationExtras(patchManagerDNF, out+"Operation aborted.\n", 1, []string{"foo.x86_64"})
	if !ok || strings.Join(extras, ",") != "bar.x86_64,baz.noarch (removing)" {
		t.Fatalf("extras = %v ok = %t", extras, ok)
	}
	if _, ok := patchSimulationExtras(patchManagerDNF, out+"Error: nothing provides x\n", 1, nil); ok {
		t.Fatal("a failed dnf run counted as a valid simulation")
	}
	if _, ok := patchSimulationExtras(patchManagerAPT, "E: broken", 100, nil); ok {
		t.Fatal("a failed apt simulation counted as valid")
	}
}

func TestPatchUnitDescriptionRoundTripAndCap(t *testing.T) {
	all, names := parsePatchUnitDescription(patchUnitDescription(true, []string{"libssl3", "bad;rm", "vim"}))
	if !all || strings.Join(names, " ") != "libssl3 vim" {
		t.Fatalf("round trip = %t %v", all, names)
	}
	long := make([]string, 500)
	for i := range long {
		long[i] = "package-with-a-long-name"
	}
	if d := patchUnitDescription(false, long); len(d) > patchUnitDescriptionMax {
		t.Fatalf("description %d bytes, over the cap", len(d))
	}
}
