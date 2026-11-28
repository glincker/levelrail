package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx/rds"
	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/store"
)

type rdsInstanceWire struct {
	ID            string            `json:"id"`
	ARN           string            `json:"arn"`
	Engine        string            `json:"engine"`
	EngineVersion string            `json:"engine_version"`
	Class         string            `json:"class"`
	Status        string            `json:"status"`
	Host          string            `json:"host"`
	Port          int               `json:"port"`
	AllocatedGB   int               `json:"allocated_gb"`
	MultiAZ       bool              `json:"multi_az"`
	ClusterID     string            `json:"cluster_id,omitempty"`
	SecretARN     string            `json:"secret_arn,omitempty"`
	Managed       bool              `json:"managed"`
	Tags          map[string]string `json:"tags"`
	MonthlyUSD    *float64          `json:"est_monthly_usd"`
}

func toRDSWire(i rds.Instance, price *float64) rdsInstanceWire {
	tags := i.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	return rdsInstanceWire{ID: i.ID, ARN: i.ARN, Engine: i.Engine, EngineVersion: i.EngineVersion, Class: i.Class, Status: i.Status,
		Host: i.Host, Port: i.Port, AllocatedGB: i.AllocatedGB, MultiAZ: i.MultiAZ, ClusterID: i.ClusterID, SecretARN: i.SecretARN,
		Managed: i.Managed(), Tags: tags, MonthlyUSD: price}
}

var rdsPriceEngines = map[string]string{
	"postgres": "PostgreSQL", "mysql": "MySQL", "mariadb": "MariaDB",
	"aurora-postgresql": "Aurora PostgreSQL", "aurora-mysql": "Aurora MySQL",
}

// rdsPrice is a best effort monthly instance price; nil means the price list could not answer.
func (rt *Router) rdsPrice(ctx context.Context, a store.CloudAccount, region, class, engine string, multiAZ bool) *float64 {
	name, ok := rdsPriceEngines[engine]
	if !ok || class == "" {
		return nil
	}
	rd, err := rt.cloudSpendReader(ctx, a)
	if err != nil {
		return nil
	}
	pe, ok := rd.(interface {
		EstimatePrice(context.Context, cloudspend.PriceQuery) (cloudspend.PriceEstimate, error)
	})
	if !ok {
		return nil
	}
	deploy := "Single-AZ"
	if multiAZ {
		deploy = "Multi-AZ"
	}
	est, err := pe.EstimatePrice(ctx, cloudspend.PriceQuery{ServiceCode: "AmazonRDS", Region: region, HoursPerMonth: 730,
		Filters: map[string]string{"instanceType": class, "databaseEngine": name, "deploymentOption": deploy}})
	if err != nil || est.Monthly <= 0 {
		return nil
	}
	return &est.Monthly
}

func writeRDSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rds.ErrNotManaged):
		writeError(w, http.StatusConflict, "this database is not managed here, adopt it first")
	case errors.Is(err, rds.ErrNeedsConfirm), errors.Is(err, rds.ErrNoTimer), errors.Is(err, rds.ErrRefused):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, rds.ErrNeedsRoot):
		writeError(w, http.StatusForbidden, "delete needs a root caller and a final snapshot name")
	default:
		writeAWSError(w, err)
	}
}

func (rt *Router) handleRDSList(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rds.New(c.client, c.guard).DescribeDBInstances(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	memo := map[string]*float64{}
	out := make([]rdsInstanceWire, 0, len(list))
	for _, i := range list {
		k := fmt.Sprintf("%s|%s|%t", i.Class, i.Engine, i.MultiAZ)
		p, seen := memo[k]
		if !seen {
			p = rt.rdsPrice(r.Context(), c.acct, c.region, i.Class, i.Engine, i.MultiAZ)
			memo[k] = p
		}
		out = append(out, toRDSWire(i, p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "instances": out})
}

func (rt *Router) handleRDSGet(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rds.New(c.client, c.guard).DescribeDBInstances(r.Context(), c.region, r.PathValue("rid"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "not found in AWS")
		return
	}
	i := list[0]
	writeJSON(w, http.StatusOK, toRDSWire(i, rt.rdsPrice(r.Context(), c.acct, c.region, i.Class, i.Engine, i.MultiAZ)))
}

func (rt *Router) handleRDSClusters(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rds.New(c.client, c.guard).DescribeDBClusters(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type clusterWire struct {
		ID        string            `json:"id"`
		Engine    string            `json:"engine"`
		Status    string            `json:"status"`
		Endpoint  string            `json:"endpoint"`
		Reader    string            `json:"reader_endpoint,omitempty"`
		Port      int               `json:"port"`
		MultiAZ   bool              `json:"multi_az"`
		Members   []string          `json:"members"`
		StoppedAt string            `json:"stopped_at,omitempty"`
		AutoStart *time.Time        `json:"auto_restart_at,omitempty"`
		SecretARN string            `json:"secret_arn,omitempty"`
		Managed   bool              `json:"managed"`
		Tags      map[string]string `json:"tags"`
	}
	out := make([]clusterWire, 0, len(list))
	for _, k := range list {
		m, t := k.Members, k.Tags
		if m == nil {
			m = []string{}
		}
		if t == nil {
			t = map[string]string{}
		}
		out = append(out, clusterWire{ID: k.ID, Engine: k.Engine, Status: k.Status, Endpoint: k.Endpoint, Reader: k.ReaderEndpoint, Port: k.Port,
			MultiAZ: k.MultiAZ, Members: m, SecretARN: k.SecretARN, Managed: k.Managed(), Tags: t, StoppedAt: t[rds.StoppedAtTag], AutoStart: timePtr(k.AutoRestartAt)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "clusters": out, "max_stop_days": rds.MaxStopDays})
}

func (rt *Router) handleRDSEstimate(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, q.Get("region"))
	if !ok {
		return
	}
	multi, _ := strconv.ParseBool(q.Get("multi_az"))
	engine, class := q.Get("engine"), q.Get("class")
	if class == "" || engine == "" {
		writeError(w, http.StatusBadRequest, "class and engine are required")
		return
	}
	cfg := rt.cloudGuardConfig()
	allowed := false
	for _, t := range cfg.InstanceTypes {
		allowed = allowed || t == class
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"class": class, "engine": engine, "multi_az": multi, "est_monthly_usd": rt.rdsPrice(r.Context(), c.acct, c.region, class, engine, multi),
		"class_allowed": allowed, "per_create_max_usd": cfg.PerCreateMaxUSD, "needs_confirm": multi || rds.IsAurora(engine),
		"default_expires_hours": int(cfg.DefaultTTL / time.Hour), "kill_switch": cfg.KillSwitch,
	})
}

type rdsActionBody struct {
	Action     string `json:"action"`
	Region     string `json:"region"`
	SnapshotID string `json:"snapshot_id"`
	Env        string `json:"env"`
	Project    string `json:"project"`
}

func (rt *Router) handleRDSAction(w http.ResponseWriter, r *http.Request) {
	var body rdsActionBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch body.Action {
	case "snapshot", "stop", "start", "adopt":
	default:
		writeError(w, http.StatusBadRequest, "action must be snapshot, stop, start or adopt")
		return
	}
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, body.Region)
	if !ok {
		return
	}
	id := r.PathValue("rid")
	cl := rds.New(c.client, c.guard)
	ctx := r.Context()
	var err error
	snap := body.SnapshotID
	switch body.Action {
	case "snapshot":
		if snap == "" {
			snap = rdsSnapshotName(id)
		}
		err = cl.CreateSnapshot(ctx, c.region, id, snap, rt.awsTagMeta(r, id, body.Env, body.Project))
	case "stop":
		err = cl.Stop(ctx, c.region, id, snap)
	case "start":
		err = cl.Start(ctx, c.region, id)
	case "adopt":
		err = cl.Adopt(ctx, c.region, id, rt.awsTagMeta(r, id, body.Env, body.Project))
	}
	if err != nil {
		rt.auditCloud(r, "cloud.aws.rds."+body.Action+"_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": id})
		writeRDSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.rds."+body.Action, map[string]any{"account": c.acct.ID, "region": c.region, "id": id, "snapshot": snap})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "action": body.Action, "snapshot_id": snap})
}

var rdsNameClean = regexp.MustCompile(`[^a-z0-9-]+`)

func rdsSnapshotName(id string) string {
	return "lr-" + rdsNameClean.ReplaceAllString(strings.ToLower(id), "-") + "-" + time.Now().UTC().Format("20060102-150405")
}

func (rt *Router) handleRDSSnapshots(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rds.New(c.client, c.guard).DescribeDBSnapshots(r.Context(), c.region, r.URL.Query().Get("instance"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type snapWire struct {
		ID          string    `json:"id"`
		InstanceID  string    `json:"instance_id"`
		Engine      string    `json:"engine"`
		Status      string    `json:"status"`
		Type        string    `json:"type"`
		AllocatedGB int       `json:"allocated_gb"`
		CreatedAt   time.Time `json:"created_at"`
		Managed     bool      `json:"managed"`
	}
	out := make([]snapWire, 0, len(list))
	for _, s := range list {
		out = append(out, snapWire{ID: s.ID, InstanceID: s.InstanceID, Engine: s.Engine, Status: s.Status, Type: s.Type, AllocatedGB: s.AllocatedGB, CreatedAt: s.CreatedAt, Managed: s.Managed()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "snapshots": out})
}

func (rt *Router) handleRDSSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	if err := rds.New(c.client, c.guard).DeleteSnapshot(r.Context(), c.region, sid); err != nil {
		writeRDSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.rds.snapshot_delete", map[string]any{"account": c.acct.ID, "region": c.region, "snapshot": sid})
	w.WriteHeader(http.StatusNoContent)
}

type rdsCreateBody struct {
	Region              string   `json:"region"`
	ID                  string   `json:"id"`
	Engine              string   `json:"engine"`
	Class               string   `json:"class"`
	MasterUsername      string   `json:"master_username"`
	DBName              string   `json:"db_name"`
	AllocatedGB         int      `json:"allocated_gb"`
	MultiAZ             bool     `json:"multi_az"`
	Cluster             bool     `json:"cluster"`
	ClusterID           string   `json:"cluster_id"`
	SubnetGroup         string   `json:"subnet_group"`
	SecurityGroupIDs    []string `json:"security_group_ids"`
	BackupRetention     int      `json:"backup_retention"`
	Confirm             bool     `json:"confirm"`
	Env                 string   `json:"env"`
	Project             string   `json:"project"`
	EstimatedMonthlyUSD float64  `json:"estimated_monthly_usd"`
}

func (rt *Router) handleRDSCreate(w http.ResponseWriter, r *http.Request) {
	var body rdsCreateBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.ID) == "" || body.Engine == "" {
		writeError(w, http.StatusBadRequest, "an id and an engine are required")
		return
	}
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, body.Region)
	if !ok {
		return
	}
	est := body.EstimatedMonthlyUSD
	if est <= 0 {
		if p := rt.rdsPrice(r.Context(), c.acct, c.region, body.Class, body.Engine, body.MultiAZ); p != nil {
			est = *p
		} else if rt.cloudGuardConfig().PerCreateMaxUSD > 0 {
			writeError(w, http.StatusConflict, "the price is unavailable, so the per create limit cannot be checked")
			return
		}
	}
	in := rds.CreateInput{Account: c.acct.ID, Region: c.region, ID: body.ID, Engine: body.Engine, Class: body.Class, MasterUsername: body.MasterUsername,
		DBName: body.DBName, AllocatedGB: body.AllocatedGB, MultiAZ: body.MultiAZ, ClusterID: body.ClusterID, SubnetGroup: body.SubnetGroup,
		SecurityGroupIDs: body.SecurityGroupIDs, BackupRetention: body.BackupRetention, Confirm: body.Confirm,
		Meta: rt.awsTagMeta(r, body.ID, body.Env, body.Project), EstimatedMonthlyUSD: est}
	cl := rds.New(c.client, c.guard)
	var err error
	if body.Cluster {
		err = cl.CreateCluster(r.Context(), in)
	} else {
		err = cl.CreateInstance(r.Context(), in)
	}
	if err != nil {
		rt.auditCloud(r, "cloud.aws.rds.create_refused", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.ID})
		writeRDSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.rds.create", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.ID, "engine": body.Engine, "class": body.Class, "cluster": body.Cluster})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": body.ID, "est_monthly_usd": est, "password_source": "secrets_manager"})
}

var rdsExternalEngines = map[string]string{"postgres": "postgres", "aurora-postgresql": "postgres", "mysql": "mysql", "aurora-mysql": "mysql", "mariadb": "mariadb"}

type rdsConnectBody struct {
	Region string `json:"region"`
	Name   string `json:"name"`
}

// handleRDSConnectAsDatabase records the endpoint as an external database. The master
// password stays in Secrets Manager; only the secret ARN is audited, never a value.
func (rt *Router) handleRDSConnectAsDatabase(w http.ResponseWriter, r *http.Request) {
	var body rdsConnectBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rt.externalDatabases == nil || rt.databases == nil {
		writeError(w, http.StatusNotImplemented, "external databases are not available")
		return
	}
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, body.Region)
	if !ok {
		return
	}
	cl := rds.New(c.client, c.guard)
	list, err := cl.DescribeDBInstances(r.Context(), c.region, r.PathValue("rid"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "not found in AWS")
		return
	}
	i := list[0]
	engine, supported := rdsExternalEngines[i.Engine]
	if !supported {
		writeError(w, http.StatusBadRequest, "this engine cannot be connected as a database")
		return
	}
	host, port, secret := i.Host, i.Port, i.SecretARN
	if i.ClusterID != "" {
		clusters, cerr := cl.DescribeDBClusters(r.Context(), c.region)
		if cerr != nil {
			writeAWSError(w, cerr)
			return
		}
		for _, k := range clusters {
			if k.ID == i.ClusterID {
				host, port, secret = k.Endpoint, k.Port, k.SecretARN
			}
		}
	}
	if host == "" {
		writeError(w, http.StatusConflict, "the instance has no endpoint yet")
		return
	}
	name := body.Name
	if name == "" {
		name = rdsNameClean.ReplaceAllString(strings.ToLower(i.ID), "-")
	}
	rt.auditCloud(r, "cloud.aws.rds.connect_as_database", map[string]any{"account": c.acct.ID, "region": c.region, "id": i.ID, "name": name, "secret_arn": secret})
	rt.createExternalDatabase(w, r, externalDatabaseRequest{Name: name, Engine: engine, Host: host, Port: port, Username: i.MasterUsername, Database: i.DBName, TLSMode: "require"})
}

// handleRDSClusterAction runs snapshot, stop or start on a managed cluster. A stopped cluster is
// started again by AWS after max_stop_days, so the response says so.
func (rt *Router) handleRDSClusterAction(w http.ResponseWriter, r *http.Request) {
	var body rdsActionBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch body.Action {
	case "snapshot", "stop", "start":
	default:
		writeError(w, http.StatusBadRequest, "action must be snapshot, stop or start")
		return
	}
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, body.Region)
	if !ok {
		return
	}
	id := r.PathValue("cid")
	cl := rds.New(c.client, c.guard)
	var err error
	snap := body.SnapshotID
	switch body.Action {
	case "snapshot":
		if snap == "" {
			snap = rdsSnapshotName(id)
		}
		err = cl.CreateClusterSnapshot(r.Context(), c.region, id, snap, rt.awsTagMeta(r, id, body.Env, body.Project))
	case "stop":
		err = cl.StopCluster(r.Context(), c.region, id)
	case "start":
		err = cl.StartCluster(r.Context(), c.region, id)
	}
	if err != nil {
		rt.auditCloud(r, "cloud.aws.rds.cluster_"+body.Action+"_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": id})
		writeRDSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.rds.cluster_"+body.Action, map[string]any{"account": c.acct.ID, "region": c.region, "id": id, "snapshot": snap})
	resp := map[string]any{"id": id, "action": body.Action, "snapshot_id": snap}
	if body.Action == "stop" {
		resp["auto_restart_days"] = rds.MaxStopDays
		resp["auto_restart_note"] = fmt.Sprintf("AWS starts a stopped cluster again after %d days", rds.MaxStopDays)
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (rt *Router) handleRDSClusterSnapshots(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := rds.New(c.client, c.guard).DescribeDBClusterSnapshots(r.Context(), c.region, r.URL.Query().Get("cluster"), "")
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type snapWire struct {
		ID          string    `json:"id"`
		ClusterID   string    `json:"cluster_id"`
		Engine      string    `json:"engine"`
		Status      string    `json:"status"`
		Type        string    `json:"type"`
		AllocatedGB int       `json:"allocated_gb"`
		CreatedAt   time.Time `json:"created_at"`
		Managed     bool      `json:"managed"`
	}
	out := make([]snapWire, 0, len(list))
	for _, s := range list {
		out = append(out, snapWire{ID: s.ID, ClusterID: s.ClusterID, Engine: s.Engine, Status: s.Status, Type: s.Type, AllocatedGB: s.AllocatedGB, CreatedAt: s.CreatedAt, Managed: s.Managed()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "snapshots": out})
}

func (rt *Router) handleRDSClusterSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, rds.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	sid := r.PathValue("sid")
	if err := rds.New(c.client, c.guard).DeleteClusterSnapshot(r.Context(), c.region, sid); err != nil {
		writeRDSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.rds.cluster_snapshot_delete", map[string]any{"account": c.acct.ID, "region": c.region, "snapshot": sid})
	w.WriteHeader(http.StatusNoContent)
}
