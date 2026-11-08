package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	patchUnitProps = "LoadState,ActiveState,SubState,Result,ExecMainStatus"
	// patchUnitDescriptionMax keeps the unit description short; names past it are left out of the record.
	patchUnitDescriptionMax = 2048
	maxPatchAdoptLimit      = 200
)

var (
	patchSystemActor = accessActor{kind: "system", id: "node-patch-adoption", name: "node patch adoption"}
	aptRemvLine      = regexp.MustCompile(`^Remv (\S+)`)
	patchRPMArches   = map[string]bool{"x86_64": true, "aarch64": true, "noarch": true, "i686": true, "armv7hl": true, "ppc64le": true, "s390x": true}
)

// StartNodePatchAdoption picks up upgrade units a previous control plane process left behind; later calls are no-ops.
func (rt *Router) StartNodePatchAdoption(ctx context.Context) {
	rt.startNodePatchAdoption(ctx, rt.buildPatcher())
}

func (rt *Router) startNodePatchAdoption(ctx context.Context, p *nodePatcher) {
	p.adoptOnce.Do(func() {
		if !p.runner.UnderSystemd() {
			p.markScanned()
			return
		}
		go rt.adoptPatchUnits(context.WithoutCancel(ctx), p)
	})
}

func (rt *Router) patchBrandShort() string {
	if rt.brand != nil {
		return rt.brand.ShortName
	}
	return ""
}

func (p *nodePatcher) markScanned() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.scanDone {
		p.scanDone = true
		close(p.scanned)
	}
}

// nodeBusyLocked counts a tracked unit too: a timed out or adopted upgrade may still be running apt.
func (p *nodePatcher) nodeBusyLocked(id string) bool {
	if p.busy[id] || !p.scanDone {
		return true
	}
	for _, n := range p.live {
		if n == id {
			return true
		}
	}
	return false
}

func (p *nodePatcher) track(key, node string) {
	p.mu.Lock()
	p.live[key] = node
	p.mu.Unlock()
}

func (p *nodePatcher) untrack(key string) {
	p.mu.Lock()
	delete(p.live, key)
	p.mu.Unlock()
}

func (p *nodePatcher) hasRun(node, id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.runs[node] {
		if r.ID == id {
			return true
		}
	}
	return false
}

func (p *nodePatcher) adoptLimit(ctx context.Context, rt *Router) int {
	if p.adoptMax > 0 {
		return p.adoptMax
	}
	return min(max(rt.runtimeSettings.Int(ctx, runtimesettings.KeyNodesPatchAdoptLimit), 1), maxPatchAdoptLimit)
}

type adoptedPatchUnit struct {
	unit string
	st   patchUnitState
	run  nodePatchRun
}

// adoptPatchUnits tracks every leftover unit before the scan closes, so no apply can start beside one.
func (rt *Router) adoptPatchUnits(ctx context.Context, p *nodePatcher) {
	found := rt.scanPatchUnits(ctx, p)
	p.markScanned()
	for _, a := range found {
		go rt.settleAdoptedUnit(ctx, p, a)
	}
}

func (rt *Router) scanPatchUnits(ctx context.Context, p *nodePatcher) []adoptedPatchUnit {
	stem := patchStem(rt.patchBrandShort())
	out, err := p.unitQuery(ctx, rt, patchBinSystemctl, "list-units", "--all", "--plain", "--no-legend", "--type=service", "--", stem+"-patch-*")
	if err != nil {
		rt.logger.Warn("api: node patch: list leftover upgrade units failed", slog.String("error", err.Error()))
		return nil
	}
	unitRe := regexp.MustCompile(`^` + regexp.QuoteMeta(stem) + `-patch-([0-9a-f]{16})\.service$`)
	limit := p.adoptLimit(ctx, rt)
	manager, _, _ := detectPatchManager(p.runner)
	node := rt.guardKey(NodeGuardrailLocalID)
	history := rt.runtimeSettings.Int(ctx, runtimesettings.KeyNodesPatchRunHistory)
	var found []adoptedPatchUnit
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		m := unitRe.FindStringSubmatch(f[0])
		if m == nil || p.hasRun(node, m[1]) {
			continue
		}
		if len(found) >= limit {
			rt.logger.Warn("api: node patch: more leftover upgrade units than the adopt limit", slog.Int("limit", limit))
			break
		}
		show, err := p.unitQuery(ctx, rt, patchBinSystemctl, "show", "--property="+patchUnitProps+",Description", "--", f[0])
		if err != nil {
			continue
		}
		includeAll, names := parsePatchUnitDescription(patchShowValue(string(show), "Description"))
		run := nodePatchRun{ID: m[1], NodeID: node, Actor: patchSystemActor.name, Manager: manager, IncludeAll: includeAll,
			Status: patchRunRunning, StartedAt: time.Now(), Packages: names, Held: []string{}}
		st := parsePatchUnitShow(string(show))
		if !st.done {
			p.track(f[0], node)
		}
		p.record(run, history)
		found = append(found, adoptedPatchUnit{unit: f[0], st: st, run: run})
	}
	return found
}

func (rt *Router) settleAdoptedUnit(ctx context.Context, p *nodePatcher, a adoptedPatchUnit) {
	st := a.st
	if !st.done {
		st, _ = p.waitPatchUnit(ctx, rt, a.unit, nil)
	}
	run := a.run
	rt.classifyPatchRun(ctx, &run, p.finishPatchUnit(ctx, rt, a.unit, st))
	rt.finishLatePatchRun(ctx, p, patchSystemActor, run)
}

// settleLatePatchRun records the real outcome of a run whose wait timed out; it never reboots.
func (rt *Router) settleLatePatchRun(ctx context.Context, p *nodePatcher, actor accessActor, run nodePatchRun, late <-chan patchInstallResult) {
	rt.classifyPatchRun(ctx, &run, <-late)
	if run.Status == patchRunSucceeded {
		run.RebootNote = "the upgrade finished after the wait timed out; no reboot was scheduled"
	}
	rt.finishLatePatchRun(ctx, p, actor, run)
}

func (rt *Router) finishLatePatchRun(ctx context.Context, p *nodePatcher, actor accessActor, run nodePatchRun) {
	finished := time.Now()
	run.FinishedAt = &finished
	p.update(run)
	rt.saveNodePatchAudit(ctx, actor, "", "", run, "finish")
	rt.logger.Info("api: node patch unit settled", slog.String("node_id", run.NodeID), slog.String("run_id", run.ID), slog.String("status", run.Status))
	if p.done != nil {
		p.done <- run.ID
	}
}

// waitPatchUnit polls until the unit ends; a nil deadline waits as long as the upgrade runs.
func (p *nodePatcher) waitPatchUnit(ctx context.Context, rt *Router, unit string, deadline <-chan time.Time) (patchUnitState, bool) {
	tick := time.NewTicker(p.pollInterval(ctx, rt))
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			return patchUnitState{}, false
		case <-tick.C:
		}
		show, err := p.unitQuery(ctx, rt, patchBinSystemctl, "show", "--property="+patchUnitProps, "--", unit)
		if err != nil {
			continue
		}
		if st := parsePatchUnitShow(string(show)); st.done {
			return st, true
		}
	}
}

// finishPatchUnit runs only for an ended unit: stopping one that still runs apt would interrupt dpkg.
func (p *nodePatcher) finishPatchUnit(ctx context.Context, rt *Router, unit string, st patchUnitState) patchInstallResult {
	lines := strconv.Itoa(max(rt.runtimeSettings.Int(ctx, runtimesettings.KeyNodesPatchOutputTailLines), 1))
	logs, _ := p.unitQuery(ctx, rt, patchBinJournalctl, "--unit="+unit, "--output=cat", "--no-pager", "--lines="+lines)
	_, _ = p.unitQuery(ctx, rt, patchBinSystemctl, "stop", "--quiet", "--", unit)
	_, _ = p.unitQuery(ctx, rt, patchBinSystemctl, "reset-failed", "--quiet", "--", unit)
	p.untrack(unit)
	res := patchInstallResult{out: string(logs), exitCode: st.exitCode}
	if st.lost {
		res.err = errors.New("the upgrade unit's final status was lost")
	}
	return res
}

func patchShowValue(out, key string) string {
	for _, line := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && k == key {
			return v
		}
	}
	return ""
}

// patchUnitDescription records the package list on the unit itself, so a restarted control plane can adopt it.
func patchUnitDescription(includeAll bool, names []string) string {
	desc := "all=" + strconv.FormatBool(includeAll) + " pkgs="
	sep := ""
	for _, n := range validPatchNames(names) {
		if len(desc)+len(sep)+len(n) > patchUnitDescriptionMax {
			break
		}
		desc += sep + n
		sep = ","
	}
	return desc
}

func parsePatchUnitDescription(desc string) (includeAll bool, names []string) {
	names = []string{}
	for _, f := range strings.Fields(desc) {
		k, v, _ := strings.Cut(f, "=")
		switch k {
		case "all":
			includeAll = v == "true"
		case "pkgs":
			for _, n := range strings.Split(v, ",") {
				if patchPackageName.MatchString(n) {
					names = append(names, n)
				}
			}
		}
	}
	return includeAll, names
}

// patchInstallSimArgv is the exact install argv in dry-run form, so the simulation resolves what the install would.
func patchInstallSimArgv(manager string, install []string) []string {
	if manager == patchManagerAPT {
		return append([]string{install[0], "-s"}, install[1:]...)
	}
	out := []string{install[0], "--assumeno"}
	for _, a := range install[1:] {
		if a != "-y" && a != "-q" {
			out = append(out, a)
		}
	}
	return out
}

// patchDependencyCheck fails closed: pinning covers the named packages, not what the resolver pulls in beside them.
func (rt *Router) patchDependencyCheck(ctx context.Context, p *nodePatcher, plan nodePatchPreviewResponse) ([]string, error) {
	sim := patchInstallSimArgv(plan.Manager, patchInstallArgv(plan.Manager, plan.bin, plan.Packages))
	sctx, cancel := context.WithTimeout(ctx, p.applyTimeout(ctx, rt))
	defer cancel()
	out, code, err := p.runner.Run(sctx, nil, sim[0], sim[1:]...)
	if err != nil {
		return nil, fmt.Errorf("the install simulation could not run: %w", err)
	}
	extras, ok := patchSimulationExtras(plan.Manager, string(out), code, patchNames(plan.Packages))
	if !ok {
		return nil, fmt.Errorf("the install simulation failed (exit %d): %s", code, lastPatchLines(string(out), 5))
	}
	return extras, nil
}

// patchSimulationExtras lists what the install would change beyond the plan; ok is false for an invalid simulation.
func patchSimulationExtras(manager, out string, code int, planned []string) (extras []string, ok bool) {
	want := make(map[string]bool, len(planned))
	for _, n := range planned {
		want[n] = true
	}
	seen := map[string]bool{}
	add := func(label string) {
		if !seen[label] {
			seen[label] = true
			extras = append(extras, label)
		}
	}
	lines := strings.Split(out, "\n")
	if manager == patchManagerAPT {
		if code != 0 {
			return nil, false
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if m := aptInstLine.FindStringSubmatch(line); m != nil && !want[m[1]] && patchPackageName.MatchString(m[1]) {
				add(m[1])
			} else if m := aptRemvLine.FindStringSubmatch(line); m != nil && patchPackageName.MatchString(m[1]) {
				add(m[1] + " (remove)")
			}
		}
		return extras, true
	}
	// dnf and yum exit 1 on --assumeno; only their own abort message marks that as a clean dry run.
	if code != 0 && (code != 1 || !strings.Contains(out, "Operation aborted")) {
		return nil, false
	}
	section := ""
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Transaction Summary") {
			break
		}
		if strings.HasSuffix(t, ":") {
			section = t
			continue
		}
		f := strings.Fields(t)
		if section == "" || len(f) < 4 || !patchRPMArches[f[1]] || !patchPackageName.MatchString(f[0]) {
			continue
		}
		name := f[0] + "." + f[1]
		switch {
		case strings.HasPrefix(section, "Removing"), strings.HasPrefix(section, "Downgrading"):
			add(name + " (" + strings.ToLower(strings.TrimSuffix(section, ":")) + ")")
		case !want[name]:
			add(name)
		}
	}
	return extras, true
}
