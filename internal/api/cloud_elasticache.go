package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx/elasticache"
)

type elastiCacheGroupWire struct {
	ID              string   `json:"id"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	NodeType        string   `json:"node_type"`
	PrimaryEndpoint string   `json:"primary_endpoint"`
	ReaderEndpoint  string   `json:"reader_endpoint,omitempty"`
	Port            int      `json:"port"`
	MultiAZ         bool     `json:"multi_az"`
	ClusterMode     bool     `json:"cluster_mode"`
	AtRestEncrypted bool     `json:"at_rest_encrypted"`
	TransitEncrypt  bool     `json:"transit_encrypted"`
	Members         []string `json:"members"`
	Managed         bool     `json:"managed"`
}

func toElastiCacheGroupWire(g elasticache.ReplicationGroup) elastiCacheGroupWire {
	m := g.Members
	if m == nil {
		m = []string{}
	}
	return elastiCacheGroupWire{ID: g.ID, Description: g.Description, Status: g.Status, NodeType: g.NodeType,
		PrimaryEndpoint: g.PrimaryEndpoint, ReaderEndpoint: g.ReaderEndpoint, Port: g.Port, MultiAZ: g.MultiAZ,
		ClusterMode: g.ClusterMode, AtRestEncrypted: g.AtRestEncrypted, TransitEncrypt: g.TransitEncrypted, Members: m, Managed: g.Managed}
}

type elastiCacheClusterWire struct {
	ID            string `json:"id"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	Status        string `json:"status"`
	NodeType      string `json:"node_type"`
	Endpoint      string `json:"endpoint"`
	Port          int    `json:"port"`
	Nodes         int    `json:"nodes"`
	GroupID       string `json:"replication_group_id,omitempty"`
	Managed       bool   `json:"managed"`
}

func toElastiCacheClusterWire(k elasticache.Cluster) elastiCacheClusterWire {
	return elastiCacheClusterWire{ID: k.ID, Engine: k.Engine, EngineVersion: k.EngineVersion, Status: k.Status, NodeType: k.NodeType,
		Endpoint: k.Endpoint, Port: k.Port, Nodes: k.Nodes, GroupID: k.GroupID, Managed: k.Managed}
}

func writeElastiCacheError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, elasticache.ErrNotManaged):
		writeError(w, http.StatusConflict, "this cache is not managed here")
	case errors.Is(err, elasticache.ErrNeedsConfirm):
		writeError(w, http.StatusBadRequest, "confirm must repeat the resource id")
	default:
		writeAWSError(w, err)
	}
}

func (rt *Router) handleElastiCacheGroups(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := elasticache.New(c.client, c.guard).DescribeReplicationGroups(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]elastiCacheGroupWire, 0, len(list))
	for _, g := range list {
		out = append(out, toElastiCacheGroupWire(g))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "replication_groups": out})
}

func (rt *Router) handleElastiCacheGroup(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := elasticache.New(c.client, c.guard).DescribeReplicationGroups(r.Context(), c.region, r.PathValue("rid"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "not found in AWS")
		return
	}
	writeJSON(w, http.StatusOK, toElastiCacheGroupWire(list[0]))
}

func (rt *Router) handleElastiCacheClusters(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := elasticache.New(c.client, c.guard).DescribeCacheClusters(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]elastiCacheClusterWire, 0, len(list))
	for _, k := range list {
		out = append(out, toElastiCacheClusterWire(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "clusters": out})
}

func (rt *Router) handleElastiCacheCluster(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := elasticache.New(c.client, c.guard).DescribeCacheClusters(r.Context(), c.region, r.PathValue("cid"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	if len(list) == 0 {
		writeError(w, http.StatusNotFound, "not found in AWS")
		return
	}
	writeJSON(w, http.StatusOK, toElastiCacheClusterWire(list[0]))
}

func (rt *Router) handleElastiCacheReboot(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	id := r.PathValue("cid")
	if err := elasticache.New(c.client, c.guard).RebootCluster(r.Context(), c.region, id); err != nil {
		rt.auditCloud(r, "cloud.aws.elasticache.reboot_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": id})
		writeElastiCacheError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.elasticache.reboot", map[string]any{"account": c.acct.ID, "region": c.region, "id": id})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "action": "reboot"})
}

func (rt *Router) handleElastiCacheSnapshots(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := elasticache.New(c.client, c.guard).DescribeSnapshots(r.Context(), c.region, r.URL.Query().Get("replication_group"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type snapWire struct {
		Name      string    `json:"name"`
		GroupID   string    `json:"replication_group_id,omitempty"`
		ClusterID string    `json:"cluster_id,omitempty"`
		Engine    string    `json:"engine"`
		Status    string    `json:"status"`
		Source    string    `json:"source"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]snapWire, 0, len(list))
	for _, s := range list {
		out = append(out, snapWire{Name: s.Name, GroupID: s.GroupID, ClusterID: s.ClusterID, Engine: s.Engine, Status: s.Status, Source: s.Source, CreatedAt: s.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "snapshots": out})
}

type elastiCacheSnapshotBody struct {
	Region           string `json:"region"`
	ReplicationGroup string `json:"replication_group"`
	Name             string `json:"name"`
	Env              string `json:"env"`
	Project          string `json:"project"`
}

func (rt *Router) handleElastiCacheSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	var body elastiCacheSnapshotBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.ReplicationGroup) == "" {
		writeError(w, http.StatusBadRequest, "a replication_group is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, body.Region)
	if !ok {
		return
	}
	name := body.Name
	if name == "" {
		name = rdsSnapshotName(body.ReplicationGroup)
	}
	err := elasticache.New(c.client, c.guard).CreateSnapshot(r.Context(), c.region, body.ReplicationGroup, name,
		rt.awsTagMeta(r, body.ReplicationGroup, body.Env, body.Project))
	if err != nil {
		rt.auditCloud(r, "cloud.aws.elasticache.snapshot_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.ReplicationGroup})
		writeElastiCacheError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.elasticache.snapshot", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.ReplicationGroup, "snapshot": name})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": body.ReplicationGroup, "snapshot_name": name})
}

// deleteElastiCache is shared by both delete routes. The kill switch blocks it like any guarded change.
func (rt *Router) deleteElastiCache(w http.ResponseWriter, r *http.Request, id string, group bool) {
	c, ok := rt.awsCallFor(w, r, elasticache.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	if rt.cloudGuardConfig().KillSwitch {
		writeError(w, http.StatusConflict, "the cloud guard kill switch is on")
		return
	}
	q := r.URL.Query()
	in := elasticache.DeleteInput{Region: c.region, ID: id, Confirm: q.Get("confirm"), FinalSnapshotID: q.Get("final_snapshot")}
	cl := elasticache.New(c.client, c.guard)
	var err error
	if group {
		err = cl.DeleteReplicationGroup(r.Context(), in)
	} else {
		err = cl.DeleteCluster(r.Context(), in)
	}
	if err != nil {
		rt.auditCloud(r, "cloud.aws.elasticache.delete_refused", map[string]any{"account": c.acct.ID, "region": c.region, "id": id})
		writeElastiCacheError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.elasticache.delete", map[string]any{"account": c.acct.ID, "region": c.region, "id": id, "final_snapshot": in.FinalSnapshotID})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "action": "delete"})
}

func (rt *Router) handleElastiCacheGroupDelete(w http.ResponseWriter, r *http.Request) {
	rt.deleteElastiCache(w, r, r.PathValue("rid"), true)
}

func (rt *Router) handleElastiCacheClusterDelete(w http.ResponseWriter, r *http.Request) {
	rt.deleteElastiCache(w, r, r.PathValue("cid"), false)
}
