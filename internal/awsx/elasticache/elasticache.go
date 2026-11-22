// Package elasticache talks to ElastiCache (Redis and Valkey) over the signed
// Query API (version 2015-02-02). Reads are open; changes need the managed tag.
package elasticache

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
	"github.com/GLINCKER/levelrail/internal/cloudspend"
)

// Service is the ElastiCache Query API.
var Service = awsx.Service{Name: "elasticache", Prefix: "elasticache", SigningName: "elasticache"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "elasticache"

const version = "2015-02-02"

// Errors the API layer maps to HTTP statuses.
var (
	ErrNotManaged   = errors.New("elasticache: resource is not managed by this control plane")
	ErrNeedsConfirm = errors.New("elasticache: delete needs the resource id typed as confirmation")
)

var tagPrefix = strings.TrimSuffix(cloudspend.ProjectTagKey, "project")

// ManagedTag marks resources this control plane owns.
var ManagedTag = tagPrefix + "managed"

// Guard supplies the mandatory tags.
type Guard interface {
	Tags(m cloudguard.TagMeta) map[string]string
}

// Meta carries the values behind the mandatory tags.
type Meta = cloudguard.TagMeta

// Client is the lazy ElastiCache module; it makes no call until a method runs.
type Client struct {
	aws   *awsx.Client
	guard Guard
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard) *Client { return &Client{aws: c, guard: g} }

func notFound(op, code string) error {
	return &awsx.Error{Service: ServiceName, Op: op, Code: code, Status: 404, Class: awsx.ClassNotFound}
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ReplicationGroup is the read view of one replication group.
type ReplicationGroup struct {
	ID, ARN, Description, Status, NodeType string
	PrimaryEndpoint, ReaderEndpoint        string
	Port                                   int
	MultiAZ, ClusterMode                   bool
	AtRestEncrypted, TransitEncrypted      bool
	Members                                []string
	Managed                                bool
}

type xGroup struct {
	ID       string   `xml:"ReplicationGroupId"`
	ARN      string   `xml:"ARN"`
	Desc     string   `xml:"Description"`
	Status   string   `xml:"Status"`
	NodeType string   `xml:"CacheNodeType"`
	Primary  string   `xml:"NodeGroups>NodeGroup>PrimaryEndpoint>Address"`
	Port     int      `xml:"NodeGroups>NodeGroup>PrimaryEndpoint>Port"`
	Config   string   `xml:"ConfigurationEndpoint>Address"`
	CfgPort  int      `xml:"ConfigurationEndpoint>Port"`
	Reader   string   `xml:"NodeGroups>NodeGroup>ReaderEndpoint>Address"`
	MultiAZ  string   `xml:"MultiAZ"`
	Enabled  bool     `xml:"ClusterEnabled"`
	AtRest   bool     `xml:"AtRestEncryptionEnabled"`
	Transit  bool     `xml:"TransitEncryptionEnabled"`
	Members  []string `xml:"MemberClusters>ClusterId"`
}

func (x xGroup) toGroup() ReplicationGroup {
	g := ReplicationGroup{ID: x.ID, ARN: x.ARN, Description: x.Desc, Status: x.Status, NodeType: x.NodeType,
		PrimaryEndpoint: x.Primary, ReaderEndpoint: x.Reader, Port: x.Port, MultiAZ: x.MultiAZ == "enabled",
		ClusterMode: x.Enabled, AtRestEncrypted: x.AtRest, TransitEncrypted: x.Transit, Members: x.Members}
	if g.PrimaryEndpoint == "" {
		g.PrimaryEndpoint, g.Port = x.Config, x.CfgPort
	}
	return g
}

// DescribeReplicationGroups lists replication groups (all pages), or the one named.
// Ownership is read from tags with one extra call per group.
func (c *Client) DescribeReplicationGroups(ctx context.Context, region string, id ...string) ([]ReplicationGroup, error) {
	var res []ReplicationGroup
	marker := ""
	for {
		v := url.Values{}
		if len(id) > 0 && id[0] != "" {
			v.Set("ReplicationGroupId", id[0])
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items  []xGroup `xml:"DescribeReplicationGroupsResult>ReplicationGroups>ReplicationGroup"`
			Marker string   `xml:"DescribeReplicationGroupsResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeReplicationGroups", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			g := x.toGroup()
			managed, err := c.managed(ctx, region, g.ARN)
			if err != nil {
				return nil, err
			}
			g.Managed = managed
			res = append(res, g)
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

// Cluster is the read view of one cache cluster (a node or a standalone cluster).
type Cluster struct {
	ID, ARN, Engine, EngineVersion, Status, NodeType string
	Endpoint                                         string
	Port, Nodes                                      int
	GroupID                                          string
	NodeIDs                                          []string
	Managed                                          bool
}

// DescribeCacheClusters lists cache clusters (all pages), or the one named.
func (c *Client) DescribeCacheClusters(ctx context.Context, region string, id ...string) ([]Cluster, error) {
	var res []Cluster
	marker := ""
	for {
		v := url.Values{"ShowCacheNodeInfo": {"true"}}
		if len(id) > 0 && id[0] != "" {
			v.Set("CacheClusterId", id[0])
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				ID       string   `xml:"CacheClusterId"`
				ARN      string   `xml:"ARN"`
				Engine   string   `xml:"Engine"`
				Version  string   `xml:"EngineVersion"`
				Status   string   `xml:"CacheClusterStatus"`
				NodeType string   `xml:"CacheNodeType"`
				Nodes    int      `xml:"NumCacheNodes"`
				Group    string   `xml:"ReplicationGroupId"`
				Config   string   `xml:"ConfigurationEndpoint>Address"`
				NodeAddr string   `xml:"CacheNodes>CacheNode>Endpoint>Address"`
				Port     int      `xml:"CacheNodes>CacheNode>Endpoint>Port"`
				NodeIDs  []string `xml:"CacheNodes>CacheNode>CacheNodeId"`
			} `xml:"DescribeCacheClustersResult>CacheClusters>CacheCluster"`
			Marker string `xml:"DescribeCacheClustersResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeCacheClusters", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			ep := x.Config
			if ep == "" {
				ep = x.NodeAddr
			}
			managed, err := c.managed(ctx, region, x.ARN)
			if err != nil {
				return nil, err
			}
			res = append(res, Cluster{ID: x.ID, ARN: x.ARN, Engine: x.Engine, EngineVersion: x.Version, Status: x.Status,
				NodeType: x.NodeType, Endpoint: ep, Port: x.Port, Nodes: x.Nodes, GroupID: x.Group, NodeIDs: x.NodeIDs, Managed: managed})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

type xTag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

// ListTags reads the tags of a resource ARN.
func (c *Client) ListTags(ctx context.Context, region, arn string) (map[string]string, error) {
	var out struct {
		Tags []xTag `xml:"ListTagsForResourceResult>TagList>Tag"`
	}
	if err := c.aws.Query(ctx, Service, region, "ListTagsForResource", version, url.Values{"ResourceName": {arn}}, &out); err != nil {
		return nil, err
	}
	m := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		m[t.Key] = t.Value
	}
	return m, nil
}

func (c *Client) managed(ctx context.Context, region, arn string) (bool, error) {
	if arn == "" {
		return false, nil
	}
	tags, err := c.ListTags(ctx, region, arn)
	if err != nil {
		return false, err
	}
	return tags[ManagedTag] == "true", nil
}

// Snapshot is the read view of one snapshot.
type Snapshot struct {
	Name, ARN, GroupID, ClusterID, Engine, Status, Source string
	CreatedAt                                             time.Time
}

// DescribeSnapshots lists snapshots (all pages), optionally for one replication group.
func (c *Client) DescribeSnapshots(ctx context.Context, region, groupID string) ([]Snapshot, error) {
	var res []Snapshot
	marker := ""
	for {
		v := url.Values{}
		if groupID != "" {
			v.Set("ReplicationGroupId", groupID)
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				Name    string `xml:"SnapshotName"`
				ARN     string `xml:"ARN"`
				Group   string `xml:"ReplicationGroupId"`
				Cluster string `xml:"CacheClusterId"`
				Engine  string `xml:"Engine"`
				Status  string `xml:"SnapshotStatus"`
				Source  string `xml:"SnapshotSource"`
				Created string `xml:"NodeSnapshots>NodeSnapshot>SnapshotCreateTime"`
			} `xml:"DescribeSnapshotsResult>Snapshots>Snapshot"`
			Marker string `xml:"DescribeSnapshotsResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeSnapshots", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Snapshot{Name: x.Name, ARN: x.ARN, GroupID: x.Group, ClusterID: x.Cluster, Engine: x.Engine,
				Status: x.Status, Source: x.Source, CreatedAt: parseTime(x.Created)})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

func (c *Client) requireManagedGroup(ctx context.Context, region, id string) error {
	list, err := c.DescribeReplicationGroups(ctx, region, id)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return notFound("DescribeReplicationGroups", "ReplicationGroupNotFoundFault")
	}
	if !list[0].Managed {
		return ErrNotManaged
	}
	return nil
}

func (c *Client) requireManagedCluster(ctx context.Context, region, id string) (Cluster, error) {
	list, err := c.DescribeCacheClusters(ctx, region, id)
	if err != nil {
		return Cluster{}, err
	}
	if len(list) == 0 {
		return Cluster{}, notFound("DescribeCacheClusters", "CacheClusterNotFound")
	}
	if !list[0].Managed {
		return Cluster{}, ErrNotManaged
	}
	return list[0], nil
}

// RebootCluster reboots every node of a managed cache cluster.
func (c *Client) RebootCluster(ctx context.Context, region, id string) error {
	k, err := c.requireManagedCluster(ctx, region, id)
	if err != nil {
		return err
	}
	v := url.Values{"CacheClusterId": {id}}
	for i, n := range k.NodeIDs {
		v.Set("CacheNodeIdsToReboot.CacheNodeId."+strconv.Itoa(i+1), n)
	}
	return c.aws.Query(ctx, Service, region, "RebootCacheCluster", version, v, nil)
}

// DeleteInput is a delete request. Confirm must repeat the resource id.
type DeleteInput struct {
	Region, ID, Confirm, FinalSnapshotID string
}

func (in DeleteInput) confirmed() bool { return in.ID != "" && in.Confirm == in.ID }

// DeleteReplicationGroup deletes a managed replication group, with an optional final snapshot.
func (c *Client) DeleteReplicationGroup(ctx context.Context, in DeleteInput) error {
	if !in.confirmed() {
		return ErrNeedsConfirm
	}
	if err := c.requireManagedGroup(ctx, in.Region, in.ID); err != nil {
		return err
	}
	v := url.Values{"ReplicationGroupId": {in.ID}}
	if in.FinalSnapshotID != "" {
		v.Set("FinalSnapshotIdentifier", in.FinalSnapshotID)
	}
	return c.aws.Query(ctx, Service, in.Region, "DeleteReplicationGroup", version, v, nil)
}

// DeleteCluster deletes a managed standalone cache cluster, with an optional final snapshot.
func (c *Client) DeleteCluster(ctx context.Context, in DeleteInput) error {
	if !in.confirmed() {
		return ErrNeedsConfirm
	}
	if _, err := c.requireManagedCluster(ctx, in.Region, in.ID); err != nil {
		return err
	}
	v := url.Values{"CacheClusterId": {in.ID}}
	if in.FinalSnapshotID != "" {
		v.Set("FinalSnapshotIdentifier", in.FinalSnapshotID)
	}
	return c.aws.Query(ctx, Service, in.Region, "DeleteCacheCluster", version, v, nil)
}

// CreateSnapshot snapshots a managed replication group and tags the snapshot as managed.
func (c *Client) CreateSnapshot(ctx context.Context, region, groupID, name string, m Meta) error {
	if err := c.requireManagedGroup(ctx, region, groupID); err != nil {
		return err
	}
	v := url.Values{"ReplicationGroupId": {groupID}, "SnapshotName": {name}}
	for i, t := range awsx.TagList(c.guard.Tags(m)) {
		p := "Tags.Tag." + strconv.Itoa(i+1)
		v.Set(p+".Key", t.Key)
		v.Set(p+".Value", t.Value)
	}
	return c.aws.Query(ctx, Service, region, "CreateSnapshot", version, v, nil)
}
