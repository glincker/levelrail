package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
)

// Doctor groups, in display order.
const (
	doctorGroupRuntime  = "runtime"
	doctorGroupHost     = "host"
	doctorGroupNetwork  = "network"
	doctorGroupSecurity = "security"
	doctorGroupBackups  = "backups"
	doctorGroupNodes    = "nodes"
	doctorGroupHardware = "hardware"
)

// doctorParallelism bounds how many checks run at once.
const doctorParallelism = 8

// defaultDoctorCheckTimeout bounds one check; a slow one reports unknown instead of holding the page.
const defaultDoctorCheckTimeout = 10 * time.Second

// envDoctorCheckTimeout overrides defaultDoctorCheckTimeout.
const envDoctorCheckTimeout = "APP_DOCTOR_CHECK_TIMEOUT"

// doctorFixLookupTimeout bounds attaching typed fixes to a finished run.
const doctorFixLookupTimeout = 10 * time.Second

// doctorCheckDef is one registered check. A def may emit several results
// (one per node, port or registry host).
type doctorCheckDef struct {
	ID      string
	Group   string
	Timeout time.Duration
	Run     func(ctx context.Context) []doctorCheckResource
}

func (rt *Router) doctorDefs() []doctorCheckDef {
	httpPort, httpsPort := rt.doctorHTTPPort, rt.doctorHTTPSPort
	if httpPort == 0 {
		httpPort = defaultDoctorHTTPPort
	}
	if httpsPort == 0 {
		httpsPort = defaultDoctorHTTPSPort
	}
	one := func(id, group string, f func(ctx context.Context) doctorCheckResource) doctorCheckDef {
		return doctorCheckDef{ID: id, Group: group, Run: func(ctx context.Context) []doctorCheckResource {
			return []doctorCheckResource{f(ctx)}
		}}
	}
	many := func(id, group string, f func(ctx context.Context) []doctorCheckResource) doctorCheckDef {
		return doctorCheckDef{ID: id, Group: group, Run: f}
	}
	return []doctorCheckDef{
		one("docker", doctorGroupRuntime, rt.relabelEngine(rt.doctorCheckDocker)),
		one("install_state", doctorGroupRuntime, rt.doctorCheckInstallState),
		one("container_runtime", doctorGroupRuntime, rt.doctorCheckContainerRuntime),
		one("docker_guard", doctorGroupRuntime, rt.relabelEngine(rt.doctorCheckDockerGuard)),
		one("docker_privilege", doctorGroupRuntime, rt.relabelEngine(rt.doctorCheckDockerPrivilege)),
		one("container_hardening", doctorGroupRuntime, rt.doctorCheckHardeningNow),
		one("network_pool", doctorGroupRuntime, rt.doctorCheckNetworkPool),
		one("docker_orphans", doctorGroupRuntime, rt.doctorCheckOrphans),

		one("disk_space", doctorGroupHost, func(context.Context) doctorCheckResource { return rt.doctorCheckDiskSpace() }),
		one("inodes", doctorGroupHost, rt.doctorCheckInodes),
		one("docker_root_disk", doctorGroupHost, rt.doctorCheckDockerRootDisk),
		one("disk_hygiene", doctorGroupHost, rt.doctorCheckDiskHygiene),
		one("data_dir_writable", doctorGroupHost, func(context.Context) doctorCheckResource { return rt.doctorCheckDataDirWritable() }),
		one("ram", doctorGroupHost, func(context.Context) doctorCheckResource { return rt.doctorCheckRAM() }),
		one("cpu", doctorGroupHost, func(context.Context) doctorCheckResource { return rt.doctorCheckCPU() }),
		one("disk_io_latency", doctorGroupHost, func(context.Context) doctorCheckResource { return rt.doctorCheckDiskIOLatency() }),
		one("database", doctorGroupHost, rt.doctorCheckDatabase),
		one("idle_footprint", doctorGroupHost, rt.doctorCheckIdleFootprint),

		one("port_http", doctorGroupNetwork, func(ctx context.Context) doctorCheckResource { return rt.doctorCheckPortCtx(ctx, httpPort) }),
		one("port_https", doctorGroupNetwork, func(ctx context.Context) doctorCheckResource { return rt.doctorCheckPortCtx(ctx, httpsPort) }),
		one("firewall", doctorGroupNetwork, func(ctx context.Context) doctorCheckResource { return doctorCheckFirewallCtx(ctx) }),
		many("network", doctorGroupNetwork, func(ctx context.Context) []doctorCheckResource {
			return rt.doctorRunNetworkChecks(ctx, httpPort, httpsPort)
		}),
		one("dns_domains", doctorGroupNetwork, rt.doctorCheckDomainDNS),
		one("dashboard_https", doctorGroupNetwork, rt.doctorCheckDashboardHTTPS),
		many("registry", doctorGroupNetwork, rt.doctorCheckRegistryReachability),
		many("cross_node_ingress", doctorGroupNetwork, rt.doctorCheckCrossNodeIngress),
		many("ingress_edge", doctorGroupNetwork, func(context.Context) []doctorCheckResource { return rt.doctorCheckIngressEdge() }),
		many("ingress_survives", doctorGroupNetwork, func(context.Context) []doctorCheckResource {
			return rt.doctorCheckIngressSurvives(time.Now())
		}),
		many("exposure", doctorGroupNetwork, rt.doctorCheckExposure),
		one("certificates", doctorGroupNetwork, rt.doctorCheckCertExpiry),

		one("master_key_rotation", doctorGroupSecurity, rt.doctorCheckMasterKeyRotation),
		one("secret_binding", doctorGroupSecurity, rt.doctorCheckSecretBinding),
		one("stale_secrets", doctorGroupSecurity, rt.doctorCheckStaleSecrets),
		one("notification_channel_secrets", doctorGroupSecurity, rt.doctorCheckNotificationChannelSecrets),
		one("secret_reveal_policy", doctorGroupSecurity, func(context.Context) doctorCheckResource { return doctorCheckSecretReveal() }),
		one("legacy_tokens", doctorGroupSecurity, rt.doctorCheckLegacyTokens),
		one("git_connections", doctorGroupSecurity, rt.doctorCheckGitConnections),
		one("signature_pin", doctorGroupSecurity, func(context.Context) doctorCheckResource { return rt.doctorCheckReleaseSignature() }),

		one("control_plane_backup", doctorGroupBackups, func(context.Context) doctorCheckResource { return rt.doctorCheckControlPlaneBackup() }),
		one("control_plane_dr", doctorGroupBackups, rt.doctorCheckControlPlaneDR),
		one("backup_freshness", doctorGroupBackups, rt.doctorCheckBackupFreshness),
		one("backup_drill", doctorGroupBackups, rt.doctorCheckBackupDrill),

		many("nodes", doctorGroupNodes, rt.doctorCheckNodes),
		many("mesh", doctorGroupNodes, rt.doctorCheckMesh),

		many("gpus", doctorGroupHardware, rt.doctorCheckGPUs),
		many("gpu_placement", doctorGroupHardware, rt.doctorCheckGPUPlacement),
		many("nas_client_tools", doctorGroupHardware, rt.doctorCheckNASClientTools),
	}
}

func doctorCheckTimeout(def doctorCheckDef) time.Duration {
	if def.Timeout > 0 {
		return def.Timeout
	}
	return envDuration(envDoctorCheckTimeout, defaultDoctorCheckTimeout)
}

// runDoctorDef runs one check with its own deadline. A check that ignores
// ctx is abandoned at the deadline and reports unknown, so it can never
// block the page.
func runDoctorDef(ctx context.Context, def doctorCheckDef, log *slog.Logger) []doctorCheckResource {
	timeout := doctorCheckTimeout(def)
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type outcome struct {
		res []doctorCheckResource
		err any
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- outcome{err: r}
			}
		}()
		done <- outcome{res: def.Run(cctx)}
	}()
	var res []doctorCheckResource
	select {
	case o := <-done:
		if o.err != nil {
			log.Error("api: doctor check panicked", slog.String("check_id", def.ID), slog.Any("panic", o.err))
			res = []doctorCheckResource{{Code: def.ID, Name: def.ID, Status: doctorStatusUnknown, Message: "check crashed, see the control plane log"}}
		} else {
			res = o.res
		}
	case <-cctx.Done():
		log.Warn("api: doctor check timed out", slog.String("check_id", def.ID), slog.Duration("timeout", timeout))
		res = []doctorCheckResource{{Code: def.ID, Name: def.ID, Status: doctorStatusUnknown, Message: fmt.Sprintf("did not finish within %s", timeout)}}
	}
	for i := range res {
		res[i].CheckID = def.ID
		if res[i].Group == "" {
			res[i].Group = def.Group
		}
		if res[i].Status == doctorStatusFail || res[i].Status == doctorStatusWarn {
			res[i].Class = attention.DoctorClass(res[i].Code, res[i].Status)
		}
	}
	return res
}

// runDoctorDefs runs defs concurrently and calls emit once per finished
// def, in completion order. emit is serialized.
func (rt *Router) runDoctorDefs(ctx context.Context, defs []doctorCheckDef, emit func(def doctorCheckDef, res []doctorCheckResource)) {
	sem := make(chan struct{}, doctorParallelism)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, def := range defs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res := runDoctorDef(ctx, def, rt.logger)
			mu.Lock()
			defer mu.Unlock()
			emit(def, res)
		}()
	}
	wg.Wait()
}

// runDoctorChecks is the one report the API, the CLI and the dashboard share.
func (rt *Router) runDoctorChecks(ctx context.Context) systemDoctorResponse {
	defs := rt.doctorDefs()
	order := make(map[string]int, len(defs))
	for i, d := range defs {
		order[d.ID] = i
	}
	type batch struct {
		idx int
		res []doctorCheckResource
	}
	var batches []batch
	rt.runDoctorDefs(ctx, defs, func(d doctorCheckDef, res []doctorCheckResource) {
		batches = append(batches, batch{order[d.ID], res})
	})
	sort.Slice(batches, func(i, j int) bool { return batches[i].idx < batches[j].idx })
	checks := []doctorCheckResource{}
	for _, b := range batches {
		checks = append(checks, b.res...)
	}
	rt.attachDoctorFixes(ctx, checks)
	return systemDoctorResponse{OK: doctorAllOK(checks), Checks: checks}
}

func doctorAllOK(checks []doctorCheckResource) bool {
	for _, c := range checks {
		if c.Status == doctorStatusFail {
			return false
		}
	}
	return true
}

// attachDoctorFixes stamps FixID and FixKind on every failing check that has a typed fix.
func (rt *Router) attachDoctorFixes(ctx context.Context, checks []doctorCheckResource) {
	if rt.fixit.store == nil {
		return
	}
	fctx, cancel := context.WithTimeout(ctx, doctorFixLookupTimeout)
	defer cancel()
	views, err := rt.fixitEngine().List(fctx, "", false)
	if err != nil {
		rt.logger.Warn("api: doctor could not look up fixes", slog.String("error", err.Error()))
		return
	}
	for i := range checks {
		if checks[i].Status != doctorStatusFail && checks[i].Status != doctorStatusWarn {
			continue
		}
		for _, v := range views {
			if v.CheckID == checks[i].Code && v.Failing {
				checks[i].FixID, checks[i].FixKind = v.ID, string(v.Kind)
				break
			}
		}
	}
}

func doctorStreamResource(*http.Request) string { return "" }

type doctorDefResource struct {
	ID    string `json:"id"`
	Group string `json:"group"`
}

// handleListDoctorChecks lists the registered checks without running them.
func (rt *Router) handleListDoctorChecks(w http.ResponseWriter, _ *http.Request) {
	defs := rt.doctorDefs()
	out := make([]doctorDefResource, len(defs))
	for i, d := range defs {
		out[i] = doctorDefResource{ID: d.ID, Group: d.Group}
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": out})
}

// handleRunDoctorCheck re-runs one check, the Re-check button's backend.
func (rt *Router) handleRunDoctorCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, d := range rt.doctorDefs() {
		if d.ID != id {
			continue
		}
		res := runDoctorDef(r.Context(), d, rt.logger)
		rt.attachDoctorFixes(r.Context(), res)
		writeJSON(w, http.StatusOK, map[string]any{"id": d.ID, "checks": res})
		return
	}
	writeError(w, http.StatusNotFound, "unknown doctor check")
}

// doctorStreamEvent is one SSE message: a finished check, or the closing summary.
type doctorStreamEvent struct {
	Type   string                `json:"type"`
	ID     string                `json:"id,omitempty"`
	Checks []doctorCheckResource `json:"checks,omitempty"`
	OK     *bool                 `json:"ok,omitempty"`
	Total  int                   `json:"total,omitempty"`
}

// handleStreamDoctor streams each check as it finishes so the page paints
// early. Fixes are attached in a final "fixes" event after the slow checks.
func (rt *Router) handleStreamDoctor(w http.ResponseWriter, r *http.Request) {
	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	send := func(ev doctorStreamEvent) {
		writeSSEJSON(w, ev)
		flusher.Flush()
	}
	if rt.demo != nil {
		demo := demoDoctorReport()
		send(doctorStreamEvent{Type: "check", ID: "demo", Checks: demo.Checks})
		send(doctorStreamEvent{Type: "done", OK: &demo.OK})
		return
	}
	defs := rt.doctorDefs()
	send(doctorStreamEvent{Type: "start", Total: len(defs)})
	var all []doctorCheckResource
	rt.runDoctorDefs(r.Context(), defs, func(d doctorCheckDef, res []doctorCheckResource) {
		all = append(all, res...)
		send(doctorStreamEvent{Type: "check", ID: d.ID, Checks: res})
	})
	rt.attachDoctorFixes(r.Context(), all)
	fixed := make([]doctorCheckResource, 0)
	for _, c := range all {
		if c.FixID != "" {
			fixed = append(fixed, c)
		}
	}
	send(doctorStreamEvent{Type: "fixes", Checks: fixed})
	okAll := doctorAllOK(all)
	send(doctorStreamEvent{Type: "done", OK: &okAll})
}

func writeSSEJSON(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Default().Error("api: marshal doctor event failed", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
