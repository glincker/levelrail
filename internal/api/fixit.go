package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/serverready"
	"github.com/GLINCKER/levelrail/internal/store"
)

// settingAgentAdvertiseHost overrides APP_AGENT_ADVERTISE_HOST without a restart.
const settingAgentAdvertiseHost = "agent_advertise_host"

// FixitStore is the persistence the fix engine needs; *store.DB satisfies it.
type FixitStore interface {
	RecentFixitAttempts(ctx context.Context, fixID, subject string, limit int) ([]store.FixitAttempt, error)
	CountFixitAttemptsSince(ctx context.Context, fixID string, since time.Time) (int, error)
	RecordFixitAttempt(ctx context.Context, a store.FixitAttempt) error
	GetFixitSetting(ctx context.Context, key string) (string, bool, error)
	SetFixitSetting(ctx context.Context, key, value, actor string, now time.Time) error
}

type fixitState struct {
	store    FixitStore
	once     sync.Once
	engine   *fixit.Engine
	override atomic.Pointer[string]
	observed atomic.Pointer[edgeDetection]
	// env is injectable for tests; nil reads the real host.
	host func() fixit.HostEnv
}

// WithFixit enables the fix-it engine routes and the DB-backed advertise host.
func WithFixit(s FixitStore) Option {
	return func(rt *Router) { rt.fixit.store = s }
}

type fixitCtxKey struct{}

func withRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, fixitCtxKey{}, host)
}

// advertiseHost is the address agents dial: the saved setting first, then the env value.
func (rt *Router) advertiseHost() string {
	if p := rt.fixit.override.Load(); p != nil {
		if *p != "" {
			return *p
		}
		return rt.doctorAgentAdvertiseHost
	}
	v := ""
	if rt.fixit.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if s, ok, err := rt.fixit.store.GetFixitSetting(ctx, settingAgentAdvertiseHost); err == nil && ok {
			v = s
		}
	}
	rt.fixit.override.Store(&v)
	if v != "" {
		return v
	}
	return rt.doctorAgentAdvertiseHost
}

type fixitAttemptStore struct{ s FixitStore }

func (a fixitAttemptStore) RecentAttempts(ctx context.Context, id, subject string, limit int) ([]fixit.Attempt, error) {
	rows, err := a.s.RecentFixitAttempts(ctx, id, subject, limit)
	if err != nil {
		return nil, err
	}
	out := make([]fixit.Attempt, 0, len(rows))
	for _, r := range rows {
		out = append(out, fixit.Attempt{FixID: r.FixID, Subject: r.Subject, At: r.At, Actor: r.Actor,
			Outcome: fixit.OutcomeStatus(r.Outcome), Detail: r.Detail, Fingerprint: r.Fingerprint})
	}
	return out, nil
}

func (a fixitAttemptStore) CountAttemptsSince(ctx context.Context, id string, since time.Time) (int, error) {
	return a.s.CountFixitAttemptsSince(ctx, id, since)
}

func (a fixitAttemptStore) RecordAttempt(ctx context.Context, at fixit.Attempt) error {
	return a.s.RecordFixitAttempt(ctx, store.FixitAttempt{FixID: at.FixID, Subject: at.Subject, At: at.At,
		Actor: at.Actor, Outcome: string(at.Outcome), Detail: at.Detail, Fingerprint: at.Fingerprint})
}

func (rt *Router) fixitEngine() *fixit.Engine {
	rt.fixit.once.Do(func() {
		maxPer, _ := strconv.Atoi(os.Getenv("APP_FIXIT_MAX_PER_HOUR"))
		e := fixit.NewEngine(fixitAttemptStore{rt.fixit.store}, fixit.WithMaxPerHour(maxPer))
		for _, f := range rt.fixitFixes() {
			if err := e.Register(f); err != nil {
				rt.logger.Error("api: register fix failed", "error", err.Error())
			}
		}
		rt.fixit.engine = e
	})
	return rt.fixit.engine
}

func (rt *Router) meshCheck(id string) func(context.Context) (fixit.CheckState, error) {
	return func(context.Context) (fixit.CheckState, error) {
		for _, c := range network.ReadinessChecks(rt.meshReadinessEnv(rt.mesh != nil)) {
			if c.ID == id {
				return fixit.CheckState{Failing: c.Status != network.CheckPass, Detail: c.Detail}, nil
			}
		}
		return fixit.CheckState{}, errors.New("check not found: " + id)
	}
}

func (rt *Router) fixitFixes() []fixit.Fix {
	hostFn := rt.fixit.host
	if hostFn == nil {
		reader := fixit.SystemHostReader()
		hostFn = func() fixit.HostEnv { return fixit.DetectHostEnv(reader) }
	}
	unit := "levelrail"
	if rt.brand != nil {
		unit = strings.ToLower(rt.brand.ShortName)
	}
	return append(append(append(rt.settingsFixes(), rt.certRenewFix()), rt.housekeepingFixes()...), []fixit.Fix{
		fixit.PublicAddressFix(fixit.PublicAddressDeps{
			Check:      rt.meshCheck(network.CheckPublicAddress),
			Candidates: rt.addressCandidates,
			Save: func(ctx context.Context, host, actor string) error {
				if err := rt.fixit.store.SetFixitSetting(ctx, settingAgentAdvertiseHost, host, actor, time.Now()); err != nil {
					return err
				}
				rt.fixit.override.Store(&host)
				return nil
			},
		}),
		fixit.WireGuardPrivilegeFix(fixit.WireGuardDeps{Check: rt.meshCheck(network.CheckWireGuard), Host: hostFn, FallbackUnit: unit}),
		fixit.UDPPortFix(fixit.UDPPortDeps{
			Check: rt.meshCheck(network.CheckUDPPort), Port: network.DefaultListenPort,
			Holder: func(ctx context.Context, port int) (fixit.PortHolder, bool) {
				l, ok := serverready.HostUDPHolder(ctx, port)
				return fixit.PortHolder{Process: l.Process, PID: l.PID, Container: l.Container}, ok
			},
		}),
		fixit.DiskFix(fixit.DiskDeps{
			Free:            rt.fixitDiskFree,
			Reclaimable:     rt.fixitReclaimable,
			Prune:           rt.fixitPrune,
			WarnFreePercent: envFloat("APP_FIXIT_DISK_FREE_PERCENT", 10),
		}),
		fixit.NetworkPoolFix(fixit.NetworkPoolDeps{
			Usage: func(ctx context.Context) (bool, int, int, error) {
				_, _, pool, err := rt.networkInventory(ctx)
				return pool.Enabled, pool.InUse, pool.Capacity, err
			},
			Orphans:     rt.orphanNetworkNames,
			Prune:       rt.pruneOrphanNetworks,
			WarnPercent: networkPoolWarnPercent,
			PoolEnv:     docker.EnvNetworkPool,
		}),
		fixit.ControlPlaneBackupFix(fixit.BackupDeps{
			Check: func(context.Context) (fixit.CheckState, error) {
				c := rt.doctorCheckControlPlaneBackup()
				return fixit.CheckState{Failing: c.Status == doctorStatusWarn, Detail: c.Message}, nil
			},
			Create: rt.fixitCreateBackup,
		}),
		fixit.GuidedCheckFix(fixit.GuidedDeps{
			ID: "backup.drill.failing", CheckID: "backup_drill", Subject: "backup",
			Title: "Fix the failing restore drill",
			Why:   "A backup that cannot be restored is not a backup. The latest drill of at least one database or volume failed.",
			Risk:  fixit.RiskSafe,
			Check: rt.backupDrillFinding,
		}),
		rt.openWebPortsFix(),
	}...)
}

func (rt *Router) backupDrillFinding(ctx context.Context) (fixit.Finding, error) {
	if rt.backupProtectionStore == nil {
		return fixit.Finding{}, nil
	}
	drills, err := rt.backupProtectionStore.ListBackupDrills(ctx, "", "", "", 1000)
	if err != nil {
		return fixit.Finding{}, err
	}
	seen := map[string]bool{}
	var failed []store.BackupDrill
	for _, d := range drills {
		if d.Status == store.DrillStatusRunning {
			continue
		}
		key := d.ResourceKind + "/" + d.DatabaseName + "/" + d.ServiceName + "/" + d.VolumeName
		if seen[key] {
			continue
		}
		seen[key] = true
		if d.Status == store.DrillStatusFailed {
			failed = append(failed, d)
		}
	}
	if len(failed) == 0 {
		return fixit.Finding{Detail: "the latest restore drill of every resource passed"}, nil
	}
	cls := backup.ClassifyDrillFailure(failed[0].Error)
	return fixit.Finding{
		Failing: true,
		Detail:  fmt.Sprintf("%d resource(s) failed their latest drill; first error at %s: %s", len(failed), failed[0].Stage, failed[0].Error),
		Reason:  cls.Guidance,
	}, nil
}

func (rt *Router) fixitCreateBackup(ctx context.Context) (string, error) {
	if rt.cpBackups == nil {
		return "", fixit.ErrCannotFixHere
	}
	info, err := rt.cpBackups.Create(ctx)
	if err != nil {
		return "", err
	}
	return info.Name, nil
}

func (rt *Router) addressCandidates(ctx context.Context) []fixit.Option {
	var out []fixit.Option
	seen := map[string]bool{}
	add := func(v, label string) {
		if v != "" && !seen[v] && fixit.ValidAdvertiseHost(v) == nil {
			seen[v] = true
			out = append(out, fixit.Option{Value: v, Label: label})
		}
	}
	if h, _ := ctx.Value(fixitCtxKey{}).(string); h != "" {
		host := h
		if sh, _, err := net.SplitHostPort(h); err == nil {
			host = sh
		}
		add(host, host+" (the address you opened this page with)")
	}
	addrs, _ := net.InterfaceAddrs()
	var private []string
	for _, a := range addrs {
		p, err := netip.ParsePrefix(a.String())
		if err != nil || !p.Addr().IsGlobalUnicast() || p.Addr().Is6() {
			continue
		}
		if p.Addr().IsPrivate() {
			private = append(private, p.Addr().String())
			continue
		}
		add(p.Addr().String(), p.Addr().String()+" (public interface address)")
	}
	for _, ip := range private {
		add(ip, ip+" (private network address)")
	}
	return out
}

func (rt *Router) fixitDiskFree(context.Context) (int64, int64, error) {
	var st syscallStatfs
	return statfsBytes(rt.dataDir, &st)
}

func (rt *Router) fixitReclaimable(ctx context.Context) (fixit.DiskReclaim, error) {
	var r fixit.DiskReclaim
	if rt.dockerDiskUsage != nil {
		u, err := rt.dockerDiskUsage.DiskUsage(ctx)
		if err != nil {
			return r, err
		}
		r.ContainersBytes, r.ImagesBytes = u.ContainersReclaimableBytes, u.ImagesReclaimableBytes
		r.VolumesBytes, r.BuildCacheBytes = u.VolumesReclaimableBytes, u.BuildCacheReclaimableBytes
	}
	if names, err := rt.orphanNetworkNames(ctx); err == nil {
		r.OrphanNetworks = names
	}
	return r, nil
}

func (rt *Router) fixitPrune(ctx context.Context) (int64, error) {
	if rt.janitor != nil {
		freed, err := rt.janitorDiskFix(ctx)
		_, _ = rt.pruneOrphanNetworks(ctx)
		return freed, err
	}
	if rt.dockerPruner == nil {
		return 0, fixit.ErrCannotFixHere
	}
	keep, err := rt.desiredContainerNames(ctx)
	if err != nil {
		return 0, err
	}
	res := rt.dockerPruner.Prune(ctx, keep)
	_, _ = rt.pruneOrphanNetworks(ctx)
	freed := int64(res.ContainersReclaimedBytes + res.ImagesReclaimedBytes + res.VolumesReclaimedBytes + res.BuildCacheReclaimedBytes) //nolint:gosec // byte counts fit int64
	if len(res.Errors) > 0 {
		return freed, errors.New(strings.Join(res.Errors, "; "))
	}
	return freed, nil
}

func (rt *Router) orphanNetworkNames(ctx context.Context) ([]string, error) {
	_, views, _, err := rt.networkInventory(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range views {
		if v.Removable {
			out = append(out, v.Name)
		}
	}
	return out, nil
}

func (rt *Router) pruneOrphanNetworks(ctx context.Context) ([]string, error) {
	admin, views, _, err := rt.networkInventory(ctx)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, v := range views {
		if !v.Removable {
			continue
		}
		if ok, err := admin.RemoveNetworkIfEmpty(ctx, v.Name); err == nil && ok {
			removed = append(removed, v.Name)
		}
	}
	return removed, nil
}

func (rt *Router) fixitReady(w http.ResponseWriter) bool {
	if rt.fixit.store == nil {
		writeError(w, http.StatusNotImplemented, "the fix-it engine is not configured on this control plane")
		return false
	}
	return true
}

func (rt *Router) handleListFixes(w http.ResponseWriter, r *http.Request) {
	if !rt.fixitReady(w) {
		return
	}
	ctx := withRequestHost(r.Context(), r.Host)
	views, err := rt.fixitEngine().List(ctx, r.URL.Query().Get("subject"), r.URL.Query().Get("all") == "1")
	if err != nil {
		rt.internalError(w, "api: list fixes failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fixes": views})
}

func (rt *Router) handlePreviewFix(w http.ResponseWriter, r *http.Request) {
	if !rt.fixitReady(w) {
		return
	}
	v, err := rt.fixitEngine().Preview(withRequestHost(r.Context(), r.Host), r.PathValue("id"))
	if errors.Is(err, fixit.ErrUnknownFix) {
		writeError(w, http.StatusNotFound, "unknown fix")
		return
	}
	if err != nil {
		rt.internalError(w, "api: preview fix failed", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type applyFixRequest struct {
	Params map[string]string `json:"params"`
}

func (rt *Router) handleApplyFix(w http.ResponseWriter, r *http.Request) {
	if !rt.fixitReady(w) {
		return
	}
	var req applyFixRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	_, _, actor, _ := rt.currentActor(r)
	res, err := rt.fixitEngine().Apply(withRequestHost(r.Context(), r.Host), r.PathValue("id"), actor, req.Params)
	if errors.Is(err, fixit.ErrUnknownFix) {
		writeError(w, http.StatusNotFound, "unknown fix")
		return
	}
	if err != nil {
		rt.internalError(w, "api: apply fix failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
