package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx/cloudwatch"
	"github.com/GLINCKER/levelrail/internal/awsx/ec2snap"
	"github.com/GLINCKER/levelrail/internal/awsx/rds"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// awsMetricsCache holds metric reads for a few minutes because AWS bills each one.
var awsMetricsCache = cloudwatch.NewCache(time.Duration(envIntDefault("APP_CLOUDWATCH_CACHE_TTL_SECONDS", 300)) * time.Second)

var (
	awsIDPattern     = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)
	awsMetricPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
)

const awsMetricMaxNames = 10

func (rt *Router) guardSettingInt(key, env string, def int) int {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Int(context.Background(), key)
	}
	return envIntDefault(env, def)
}

func (rt *Router) retentionConfig() ec2snap.Retention {
	return ec2snap.Retention{
		Daily:  rt.guardSettingInt(runtimesettings.KeyCloudGuardKeepDaily, "APP_CLOUD_GUARD_SNAPSHOT_KEEP_DAILY", 7),
		Weekly: rt.guardSettingInt(runtimesettings.KeyCloudGuardKeepWeekly, "APP_CLOUD_GUARD_SNAPSHOT_KEEP_WEEKLY", 4),
	}
}

func (rt *Router) idleConfig() cloudguard.IdleConfig {
	cents := func(key, env string, def int) float64 { return float64(rt.guardSettingInt(key, env, def)) / 100 }
	return cloudguard.IdleConfig{
		CPUPercent:    float64(rt.guardSettingInt(runtimesettings.KeyCloudGuardIdleCPU, "APP_CLOUD_GUARD_IDLE_CPU_PERCENT", 5)),
		Days:          rt.guardSettingInt(runtimesettings.KeyCloudGuardIdleDays, "APP_CLOUD_GUARD_IDLE_DAYS", 7),
		EBSGBMonthUSD: cents(runtimesettings.KeyCloudGuardIdleEBSCents, "APP_CLOUD_GUARD_IDLE_EBS_GB_MONTH_CENTS", 8),
		EIPMonthUSD:   cents(runtimesettings.KeyCloudGuardIdleEIPCents, "APP_CLOUD_GUARD_IDLE_EIP_MONTH_CENTS", 365),
		ALBMonthUSD:   cents(runtimesettings.KeyCloudGuardIdleALBCents, "APP_CLOUD_GUARD_IDLE_ALB_MONTH_CENTS", 1643),
	}
}

func writeEBSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ec2snap.ErrNotManaged):
		writeError(w, http.StatusConflict, "this snapshot is not managed here, so it cannot be deleted from here")
	case errors.Is(err, ec2snap.ErrNoSource), errors.Is(err, cloudwatch.ErrBadQuery):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeAWSError(w, err)
	}
}

type ebsSnapshotWire struct {
	ID          string    `json:"id"`
	VolumeID    string    `json:"volume_id"`
	State       string    `json:"state"`
	SizeGB      int       `json:"size_gb"`
	StartedAt   time.Time `json:"started_at"`
	Description string    `json:"description"`
	Managed     bool      `json:"managed"`
	ExpiresAt   string    `json:"expires_at"`
}

func toEBSSnapshotWire(s ec2snap.Snapshot) ebsSnapshotWire {
	exp := ""
	if t := s.ExpiresAt(); !t.IsZero() {
		exp = t.Format(time.RFC3339)
	}
	return ebsSnapshotWire{ID: s.ID, VolumeID: s.VolumeID, State: s.State, SizeGB: s.SizeGB, StartedAt: s.StartTime, Description: s.Description, Managed: s.Managed(), ExpiresAt: exp}
}

func (rt *Router) handleEBSSnapshots(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, ec2snap.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	vol := r.URL.Query().Get("volume")
	if vol != "" && !awsIDPattern.MatchString(vol) {
		writeError(w, http.StatusBadRequest, "invalid volume id")
		return
	}
	list, err := ec2snap.New(c.client, c.guard).DescribeSnapshots(r.Context(), c.region, vol)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]ebsSnapshotWire, 0, len(list))
	for _, s := range list {
		out = append(out, toEBSSnapshotWire(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "snapshots": out})
}

func (rt *Router) handleEBSVolumes(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, ec2snap.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := ec2snap.New(c.client, c.guard).DescribeVolumes(r.Context(), c.region, "")
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type volWire struct {
		ID         string `json:"id"`
		SizeGB     int    `json:"size_gb"`
		Type       string `json:"type"`
		State      string `json:"state"`
		AZ         string `json:"az"`
		Encrypted  bool   `json:"encrypted"`
		InstanceID string `json:"instance_id"`
		Managed    bool   `json:"managed"`
	}
	out := make([]volWire, 0, len(list))
	for _, v := range list {
		out = append(out, volWire{v.ID, v.SizeGB, v.Type, v.State, v.AZ, v.Encrypted, v.InstanceID, v.Managed()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "volumes": out})
}

type ebsCreateBody struct {
	Region      string `json:"region"`
	InstanceID  string `json:"instance_id"`
	VolumeID    string `json:"volume_id"`
	ExcludeBoot bool   `json:"exclude_boot"`
	Description string `json:"description"`
	Env         string `json:"env"`
	Project     string `json:"project"`
}

func (rt *Router) handleEBSSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	var body ebsCreateBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	src := body.InstanceID + body.VolumeID
	if (body.InstanceID == "") == (body.VolumeID == "") || !awsIDPattern.MatchString(src) {
		writeError(w, http.StatusBadRequest, "exactly one valid instance_id or volume_id is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, ec2snap.ServiceName, body.Region)
	if !ok {
		return
	}
	if rt.cloudGuardConfig().KillSwitch {
		writeError(w, http.StatusConflict, "AWS creates are paused by the kill switch.")
		return
	}
	snaps, err := ec2snap.New(c.client, c.guard).CreateSnapshots(r.Context(), ec2snap.CreateInput{
		Region: c.region, InstanceID: body.InstanceID, VolumeID: body.VolumeID, Description: body.Description, ExcludeBoot: body.ExcludeBoot,
		Meta: rt.awsTagMeta(r, src, body.Env, body.Project),
	})
	if err != nil {
		rt.auditCloud(r, "cloud.aws.ebs.snapshot_failed", map[string]any{"account": c.acct.ID, "region": c.region, "source": src})
		writeEBSError(w, err)
		return
	}
	type created struct {
		ID       string `json:"id"`
		VolumeID string `json:"volume_id"`
	}
	out := make([]created, 0, len(snaps))
	ids := make([]string, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, created{s.ID, s.VolumeID})
		ids = append(ids, s.ID)
	}
	rt.auditCloud(r, "cloud.aws.ebs.snapshot", map[string]any{"account": c.acct.ID, "region": c.region, "source": src, "snapshots": strings.Join(ids, ",")})
	writeJSON(w, http.StatusAccepted, map[string]any{"snapshots": out})
}

func (rt *Router) handleEBSSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, ec2snap.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	if !awsIDPattern.MatchString(sid) {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}
	if err := ec2snap.New(c.client, c.guard).DeleteSnapshot(r.Context(), c.region, sid); err != nil {
		rt.auditCloud(r, "cloud.aws.ebs.snapshot_delete_refused", map[string]any{"account": c.acct.ID, "region": c.region, "snapshot": sid})
		writeEBSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.ebs.snapshot_delete", map[string]any{"account": c.acct.ID, "region": c.region, "snapshot": sid})
	w.WriteHeader(http.StatusNoContent)
}

func cacheKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (rt *Router) handleCloudWatchMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := rt.awsCallFor(w, r, cloudwatch.ServiceName, q.Get("region"))
	if !ok {
		return
	}
	inst := q.Get("instance_id")
	if !awsIDPattern.MatchString(inst) {
		writeError(w, http.StatusBadRequest, "a valid instance_id is required")
		return
	}
	names := []string{"CPUUtilization"}
	if raw := q.Get("names"); raw != "" {
		names = strings.Split(raw, ",")
	}
	if len(names) > awsMetricMaxNames {
		writeError(w, http.StatusBadRequest, "too many metric names")
		return
	}
	for _, n := range names {
		if !awsMetricPattern.MatchString(n) {
			writeError(w, http.StatusBadRequest, "invalid metric name")
			return
		}
	}
	window := time.Duration(envIntDefault("APP_CLOUDWATCH_WINDOW_HOURS", int(cloudwatch.DefaultWindow/time.Hour))) * time.Hour
	if h, err := strconv.Atoi(q.Get("hours")); err == nil && h >= 1 && h <= 24*14 {
		window = time.Duration(h) * time.Hour
	}
	period := envIntDefault("APP_CLOUDWATCH_PERIOD_SECONDS", cloudwatch.DefaultPeriod)
	// Aligning the window to the period lets repeated views share one cached read.
	end := time.Now().UTC().Truncate(time.Duration(period) * time.Second)
	start := end.Add(-window)
	qs := make([]cloudwatch.Query, len(names))
	for i, n := range names {
		qs[i] = cloudwatch.Query{ID: "m" + strconv.Itoa(i), Namespace: "AWS/EC2", Name: n, Stat: "Average", Period: period,
			Dimensions: []cloudwatch.Dimension{{Name: "InstanceId", Value: inst}}}
	}
	key := cacheKey("metrics", c.acct.ID, c.region, inst, strings.Join(names, ","), strconv.FormatInt(start.Unix(), 10), strconv.FormatInt(end.Unix(), 10), strconv.Itoa(period))
	series, cached, err := cloudwatch.Cached(awsMetricsCache, key, func() ([]cloudwatch.Series, error) {
		return cloudwatch.New(c.client).GetMetricData(r.Context(), c.region, qs, start, end)
	})
	if err != nil {
		writeEBSError(w, err)
		return
	}
	type pointWire struct {
		T time.Time `json:"t"`
		V float64   `json:"v"`
	}
	type seriesWire struct {
		Name   string      `json:"name"`
		Points []pointWire `json:"points"`
	}
	out := make([]seriesWire, 0, len(series))
	for i, s := range series {
		pts := make([]pointWire, 0, len(s.Points))
		for _, p := range s.Points {
			pts = append(pts, pointWire{p.T, p.V})
		}
		out = append(out, seriesWire{Name: names[i], Points: pts})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "instance_id": inst, "period": period, "series": out, "cached": cached})
}

func (rt *Router) handleCloudWatchLogGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := rt.awsCallFor(w, r, cloudwatch.ServiceName, q.Get("region"))
	if !ok {
		return
	}
	limit := 50
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n >= 1 && n <= 500 {
		limit = n
	}
	list, err := cloudwatch.New(c.client).DescribeLogGroups(r.Context(), c.region, q.Get("prefix"), limit)
	if err != nil {
		writeEBSError(w, err)
		return
	}
	type groupWire struct {
		Name          string `json:"name"`
		RetentionDays int    `json:"retention_days"`
		StoredBytes   int64  `json:"stored_bytes"`
	}
	out := make([]groupWire, 0, len(list))
	for _, g := range list {
		out = append(out, groupWire{g.Name, g.RetentionDays, g.StoredBytes})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "groups": out})
}

func (rt *Router) handleCloudWatchLogEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := rt.awsCallFor(w, r, cloudwatch.ServiceName, q.Get("region"))
	if !ok {
		return
	}
	group := q.Get("group")
	if !awsIDPattern.MatchString(group) {
		writeError(w, http.StatusBadRequest, "a valid log group is required")
		return
	}
	limit := 100
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n >= 1 && n <= 1000 {
		limit = n
	}
	hours := 1
	if n, err := strconv.Atoi(q.Get("hours")); err == nil && n >= 1 && n <= 24*14 {
		hours = n
	}
	end := time.Now().UTC()
	events, truncated, err := cloudwatch.New(c.client).FilterLogEvents(r.Context(), c.region, cloudwatch.FilterInput{
		Group: group, Pattern: q.Get("filter"), Start: end.Add(-time.Duration(hours) * time.Hour), End: end, Limit: limit,
	})
	if err != nil {
		writeEBSError(w, err)
		return
	}
	type eventWire struct {
		TS      time.Time `json:"ts"`
		Stream  string    `json:"stream"`
		Message string    `json:"message"`
	}
	out := make([]eventWire, 0, len(events))
	for _, e := range events {
		out = append(out, eventWire{e.Time, e.Stream, e.Message})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "events": out, "truncated": truncated})
}

type idleWire struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	Detail     string   `json:"detail"`
	Action     string   `json:"action"`
	MonthlyUSD *float64 `json:"est_monthly_usd"`
}

// idleSuggestions is read only and cached, since it reads billed metrics.
func (rt *Router) idleSuggestions(ctx context.Context, c awsCall) ([]idleWire, error) {
	cfg := rt.idleConfig()
	key := cacheKey("idle", c.acct.ID, c.region, strconv.Itoa(cfg.Days), strconv.FormatFloat(cfg.CPUPercent, 'f', 2, 64))
	list, _, err := cloudwatch.Cached(awsMetricsCache, key, func() ([]cloudguard.Suggestion, error) {
		found, ferr := cloudguard.FindIdle(ctx, c.client, cfg, c.region, rt.cloudSpendConfig().MaxPages)
		if ferr != nil {
			return nil, ferr
		}
		if rt.awsServiceOn(rds.ServiceName) {
			if dbs, derr := rds.New(c.client, c.guard).DescribeDBInstances(ctx, c.region); derr == nil {
				for _, d := range dbs {
					if d.Status == "stopped" {
						found = append(found, cloudguard.Suggestion{Kind: cloudguard.KindRDSStopped, ID: d.ID, Action: cloudguard.ActionSnapshotDelete,
							Detail: "stopped, storage still billed (" + strconv.Itoa(d.AllocatedGB) + " GiB)", MonthlyUSD: float64(d.AllocatedGB) * cfg.EBSGBMonthUSD})
					}
				}
			}
			lead := time.Duration(rt.guardSettingInt(runtimesettings.KeyCloudGuardRestopLead, "APP_CLOUD_GUARD_IDLE_RESTOP_LEAD_HOURS", 24)) * time.Hour
			if ks, kerr := rds.New(c.client, c.guard).PlanClusterRestart(ctx, c.region, lead); kerr == nil {
				for _, k := range ks {
					found = append(found, cloudguard.Suggestion{Kind: cloudguard.KindRDSRestart, ID: k.ID, Action: cloudguard.ActionRestop,
						Detail: "AWS restarts this stopped cluster at " + k.AutoRestartAt.Format(time.RFC3339)})
				}
			}
		}
		return found, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]idleWire, 0, len(list))
	for _, s := range list {
		var p *float64
		if s.MonthlyUSD > 0 {
			v := s.MonthlyUSD
			p = &v
		}
		out = append(out, idleWire{s.Kind, s.ID, s.Detail, s.Action, p})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

// handleAWSIdle lists suggestions only. No route acts on them.
func (rt *Router) handleAWSIdle(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, cloudwatch.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rt.idleSuggestions(r.Context(), c)
	if err != nil {
		writeEBSError(w, err)
		return
	}
	total := 0.0
	for _, s := range list {
		if s.MonthlyUSD != nil {
			total += *s.MonthlyUSD
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "region": c.region, "suggestions": list, "est_monthly_usd_total": total})
}

// snapshotExpiredPlan lists managed snapshots past the retention policy, for the cleanup plan.
func (rt *Router) snapshotExpiredPlan(ctx context.Context, c awsCall) []map[string]any {
	out := []map[string]any{}
	items, err := ec2snap.New(c.client, c.guard).PlanRetentionFor(ctx, c.region, rt.retentionConfig())
	if err != nil {
		return out
	}
	for _, d := range items {
		out = append(out, map[string]any{"kind": ec2snap.ReasonExpired, "id": d.ID, "volume_id": d.VolumeID, "started_at": d.StartTime, "size_gb": d.SizeGB})
	}
	return out
}
