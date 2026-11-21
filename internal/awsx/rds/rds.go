// Package rds talks to RDS and Aurora over the signed Query API (version
// 2014-10-31). Creates go through the guard; only managed resources are changed.
package rds

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

// Service is the RDS Query API (endpoint prefix and version checked against the botocore model).
var Service = awsx.Service{Name: "rds", Prefix: "rds", SigningName: "rds"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "rds"

const version = "2014-10-31"

// Errors the API layer maps to HTTP statuses.
var (
	ErrRefused      = errors.New("rds: refused")
	ErrNotManaged   = errors.New("rds: resource is not managed by this control plane")
	ErrNeedsConfirm = errors.New("rds: Multi-AZ and Aurora need an explicit confirm")
	ErrNoTimer      = errors.New("rds: a stop or teardown timer (expires tag) is required")
	ErrNeedsRoot    = errors.New("rds: delete needs a root caller and a final snapshot")
)

var tagPrefix = strings.TrimSuffix(cloudspend.ProjectTagKey, "project")

// ManagedTag marks resources this control plane owns.
var ManagedTag = tagPrefix + "managed"

// StoppedAtTag records when the control plane stopped an instance.
var StoppedAtTag = tagPrefix + "stopped_at"

// ExpiresTag is the teardown timer written by the guard.
var ExpiresTag = tagPrefix + "expires"

// Guard is the create gate and tag source.
type Guard interface {
	Check(ctx context.Context, r cloudguard.CreateRequest) cloudguard.Decision
	Tags(m cloudguard.TagMeta) map[string]string
}

// Client is the lazy RDS module; it makes no call until a method runs.
type Client struct {
	aws   *awsx.Client
	guard Guard
	now   func() time.Time
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard) *Client { return &Client{aws: c, guard: g, now: time.Now} }

// WithClock replaces the clock, for tests.
func (c *Client) WithClock(now func() time.Time) *Client { c.now = now; return c }

type xTag struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

func tagMap(in []xTag) map[string]string {
	m := make(map[string]string, len(in))
	for _, t := range in {
		m[t.Key] = t.Value
	}
	return m
}

// Instance is the read view of one DB instance.
type Instance struct {
	ID, ARN, Engine, EngineVersion, Class, Status string
	Host                                          string
	Port                                          int
	AllocatedGB                                   int
	MultiAZ, Encrypted, Public                    bool
	ClusterID, SecretARN, MasterUsername, DBName  string
	AutoRestartAt                                 time.Time
	Tags                                          map[string]string
}

// Managed reports the ownership tag.
func (i Instance) Managed() bool { return i.Tags[ManagedTag] == "true" }

type xInstance struct {
	ID       string `xml:"DBInstanceIdentifier"`
	ARN      string `xml:"DBInstanceArn"`
	Engine   string `xml:"Engine"`
	Version  string `xml:"EngineVersion"`
	Class    string `xml:"DBInstanceClass"`
	Status   string `xml:"DBInstanceStatus"`
	Host     string `xml:"Endpoint>Address"`
	Port     int    `xml:"Endpoint>Port"`
	Alloc    int    `xml:"AllocatedStorage"`
	MultiAZ  bool   `xml:"MultiAZ"`
	Enc      bool   `xml:"StorageEncrypted"`
	Public   bool   `xml:"PubliclyAccessible"`
	Cluster  string `xml:"DBClusterIdentifier"`
	Secret   string `xml:"MasterUserSecret>SecretArn"`
	User     string `xml:"MasterUsername"`
	DBName   string `xml:"DBName"`
	Restart  string `xml:"AutomaticRestartTime"`
	TagItems []xTag `xml:"TagList>Tag"`
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func (x xInstance) toInstance() Instance {
	return Instance{
		ID: x.ID, ARN: x.ARN, Engine: x.Engine, EngineVersion: x.Version, Class: x.Class, Status: x.Status,
		Host: x.Host, Port: x.Port, AllocatedGB: x.Alloc, MultiAZ: x.MultiAZ, Encrypted: x.Enc, Public: x.Public,
		ClusterID: x.Cluster, SecretARN: x.Secret, MasterUsername: x.User, DBName: x.DBName,
		AutoRestartAt: parseTime(x.Restart), Tags: tagMap(x.TagItems),
	}
}

// DescribeDBInstances lists instances (all pages), or the one named.
func (c *Client) DescribeDBInstances(ctx context.Context, region string, id ...string) ([]Instance, error) {
	var res []Instance
	marker := ""
	for {
		v := url.Values{}
		if len(id) > 0 && id[0] != "" {
			v.Set("DBInstanceIdentifier", id[0])
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items  []xInstance `xml:"DescribeDBInstancesResult>DBInstances>DBInstance"`
			Marker string      `xml:"DescribeDBInstancesResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeDBInstances", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, x.toInstance())
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

// Cluster is the read view of one DB cluster (Aurora or Multi-AZ).
type Cluster struct {
	ID, ARN, Engine, EngineVersion, Status string
	Endpoint, ReaderEndpoint               string
	Port                                   int
	MultiAZ, Encrypted                     bool
	Members                                []string
	SecretARN, MasterUsername              string
	AutoRestartAt                          time.Time
	Tags                                   map[string]string
}

// Managed reports the ownership tag.
func (k Cluster) Managed() bool { return k.Tags[ManagedTag] == "true" }

// DescribeDBClusters lists clusters (all pages), or the one named.
func (c *Client) DescribeDBClusters(ctx context.Context, region string, id ...string) ([]Cluster, error) {
	var res []Cluster
	marker := ""
	for {
		v := url.Values{}
		if len(id) > 0 && id[0] != "" {
			v.Set("DBClusterIdentifier", id[0])
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				ID       string   `xml:"DBClusterIdentifier"`
				ARN      string   `xml:"DBClusterArn"`
				Engine   string   `xml:"Engine"`
				Version  string   `xml:"EngineVersion"`
				Status   string   `xml:"Status"`
				Endpoint string   `xml:"Endpoint"`
				Reader   string   `xml:"ReaderEndpoint"`
				Port     int      `xml:"Port"`
				MultiAZ  bool     `xml:"MultiAZ"`
				Enc      bool     `xml:"StorageEncrypted"`
				Members  []string `xml:"DBClusterMembers>DBClusterMember>DBInstanceIdentifier"`
				Secret   string   `xml:"MasterUserSecret>SecretArn"`
				User     string   `xml:"MasterUsername"`
				Restart  string   `xml:"AutomaticRestartTime"`
				Tags     []xTag   `xml:"TagList>Tag"`
			} `xml:"DescribeDBClustersResult>DBClusters>DBCluster"`
			Marker string `xml:"DescribeDBClustersResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeDBClusters", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Cluster{ID: x.ID, ARN: x.ARN, Engine: x.Engine, EngineVersion: x.Version, Status: x.Status,
				Endpoint: x.Endpoint, ReaderEndpoint: x.Reader, Port: x.Port, MultiAZ: x.MultiAZ, Encrypted: x.Enc,
				Members: x.Members, SecretARN: x.Secret, MasterUsername: x.User, AutoRestartAt: parseTime(x.Restart), Tags: tagMap(x.Tags)})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

// Snapshot is the read view of one DB snapshot.
type Snapshot struct {
	ID, ARN, InstanceID, Engine, Status, Type string
	AllocatedGB                               int
	CreatedAt                                 time.Time
	Tags                                      map[string]string
}

// Managed reports the ownership tag.
func (s Snapshot) Managed() bool { return s.Tags[ManagedTag] == "true" }

// DescribeDBSnapshots lists manual and automated snapshots, optionally for one instance.
func (c *Client) DescribeDBSnapshots(ctx context.Context, region, instanceID string) ([]Snapshot, error) {
	var res []Snapshot
	marker := ""
	for {
		v := url.Values{}
		if instanceID != "" {
			v.Set("DBInstanceIdentifier", instanceID)
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				ID      string `xml:"DBSnapshotIdentifier"`
				ARN     string `xml:"DBSnapshotArn"`
				Inst    string `xml:"DBInstanceIdentifier"`
				Engine  string `xml:"Engine"`
				Status  string `xml:"Status"`
				Type    string `xml:"SnapshotType"`
				Alloc   int    `xml:"AllocatedStorage"`
				Created string `xml:"SnapshotCreateTime"`
				Tags    []xTag `xml:"TagList>Tag"`
			} `xml:"DescribeDBSnapshotsResult>DBSnapshots>DBSnapshot"`
			Marker string `xml:"DescribeDBSnapshotsResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeDBSnapshots", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Snapshot{ID: x.ID, ARN: x.ARN, InstanceID: x.Inst, Engine: x.Engine, Status: x.Status, Type: x.Type,
				AllocatedGB: x.Alloc, CreatedAt: parseTime(x.Created), Tags: tagMap(x.Tags)})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

func addTags(v url.Values, tags map[string]string) {
	for i, t := range awsx.TagList(tags) {
		p := "Tags.Tag." + strconv.Itoa(i+1)
		v.Set(p+".Key", t.Key)
		v.Set(p+".Value", t.Value)
	}
}

// Meta carries the values behind the mandatory tags.
type Meta = cloudguard.TagMeta

// requireManaged loads the instance and refuses unless it carries the ownership tag.
func (c *Client) requireManaged(ctx context.Context, region, id string) (Instance, error) {
	list, err := c.DescribeDBInstances(ctx, region, id)
	if err != nil {
		return Instance{}, err
	}
	if len(list) == 0 {
		return Instance{}, &awsx.Error{Service: "rds", Op: "DescribeDBInstances", Code: "DBInstanceNotFound", Status: 404, Class: awsx.ClassNotFound}
	}
	if !list[0].Managed() {
		return Instance{}, ErrNotManaged
	}
	return list[0], nil
}

// CreateSnapshot snapshots a managed instance and tags the snapshot as managed.
func (c *Client) CreateSnapshot(ctx context.Context, region, instanceID, snapshotID string, m Meta) error {
	if _, err := c.requireManaged(ctx, region, instanceID); err != nil {
		return err
	}
	v := url.Values{"DBInstanceIdentifier": {instanceID}, "DBSnapshotIdentifier": {snapshotID}}
	addTags(v, c.guard.Tags(m))
	return c.aws.Query(ctx, Service, region, "CreateDBSnapshot", version, v, nil)
}

// CopySnapshot copies a snapshot inside the region; the copy is tagged managed.
func (c *Client) CopySnapshot(ctx context.Context, region, source, target string, m Meta) error {
	v := url.Values{"SourceDBSnapshotIdentifier": {source}, "TargetDBSnapshotIdentifier": {target}}
	addTags(v, c.guard.Tags(m))
	return c.aws.Query(ctx, Service, region, "CopyDBSnapshot", version, v, nil)
}

// DeleteSnapshot removes a snapshot only when it carries the managed tag.
func (c *Client) DeleteSnapshot(ctx context.Context, region, snapshotID string) error {
	list, err := c.describeSnapshot(ctx, region, snapshotID)
	if err != nil {
		return err
	}
	if !list.Managed() {
		return ErrNotManaged
	}
	return c.aws.Query(ctx, Service, region, "DeleteDBSnapshot", version, url.Values{"DBSnapshotIdentifier": {snapshotID}}, nil)
}

func (c *Client) describeSnapshot(ctx context.Context, region, id string) (Snapshot, error) {
	var out struct {
		Items []struct {
			ID   string `xml:"DBSnapshotIdentifier"`
			Tags []xTag `xml:"TagList>Tag"`
		} `xml:"DescribeDBSnapshotsResult>DBSnapshots>DBSnapshot"`
	}
	if err := c.aws.Query(ctx, Service, region, "DescribeDBSnapshots", version, url.Values{"DBSnapshotIdentifier": {id}}, &out); err != nil {
		return Snapshot{}, err
	}
	if len(out.Items) == 0 {
		return Snapshot{}, &awsx.Error{Service: "rds", Op: "DescribeDBSnapshots", Code: "DBSnapshotNotFound", Status: 404, Class: awsx.ClassNotFound}
	}
	return Snapshot{ID: out.Items[0].ID, Tags: tagMap(out.Items[0].Tags)}, nil
}

// AddTags writes tags to a resource ARN.
func (c *Client) AddTags(ctx context.Context, region, arn string, tags map[string]string) error {
	v := url.Values{"ResourceName": {arn}}
	addTags(v, tags)
	return c.aws.Query(ctx, Service, region, "AddTagsToResource", version, v, nil)
}

// ListTags reads the tags of a resource ARN.
func (c *Client) ListTags(ctx context.Context, region, arn string) (map[string]string, error) {
	var out struct {
		Tags []xTag `xml:"ListTagsForResourceResult>TagList>Tag"`
	}
	if err := c.aws.Query(ctx, Service, region, "ListTagsForResource", version, url.Values{"ResourceName": {arn}}, &out); err != nil {
		return nil, err
	}
	return tagMap(out.Tags), nil
}

// Adopt writes the managed tags to an unmanaged instance. It changes nothing else.
func (c *Client) Adopt(ctx context.Context, region, id string, m Meta) error {
	list, err := c.DescribeDBInstances(ctx, region, id)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return &awsx.Error{Service: "rds", Op: "DescribeDBInstances", Code: "DBInstanceNotFound", Status: 404, Class: awsx.ClassNotFound}
	}
	tags := c.guard.Tags(m)
	// Adoption claims ownership only; it must not start a teardown timer on a running database.
	delete(tags, ExpiresTag)
	return c.AddTags(ctx, region, list[0].ARN, tags)
}

// Stop stops a managed instance and records when, so an AWS auto restart can be undone.
func (c *Client) Stop(ctx context.Context, region, id, snapshotID string) error {
	inst, err := c.requireManaged(ctx, region, id)
	if err != nil {
		return err
	}
	v := url.Values{"DBInstanceIdentifier": {id}}
	if snapshotID != "" {
		v.Set("DBSnapshotIdentifier", snapshotID)
	}
	if err := c.aws.Query(ctx, Service, region, "StopDBInstance", version, v, nil); err != nil {
		return err
	}
	return c.AddTags(ctx, region, inst.ARN, map[string]string{StoppedAtTag: c.now().UTC().Format(time.RFC3339)})
}

// Start starts a managed instance and clears the stopped marker.
func (c *Client) Start(ctx context.Context, region, id string) error {
	inst, err := c.requireManaged(ctx, region, id)
	if err != nil {
		return err
	}
	if err := c.aws.Query(ctx, Service, region, "StartDBInstance", version, url.Values{"DBInstanceIdentifier": {id}}, nil); err != nil {
		return err
	}
	return c.AddTags(ctx, region, inst.ARN, map[string]string{StoppedAtTag: ""})
}

// RestopItem is one instance AWS restarted after its maximum stop period.
type RestopItem struct {
	ID        string
	StoppedAt time.Time
}

// PlanRestop lists managed instances that we stopped, that are running again and
// whose stop is older than minAge. It never changes anything.
func (c *Client) PlanRestop(ctx context.Context, region string, minAge time.Duration) ([]RestopItem, error) {
	list, err := c.DescribeDBInstances(ctx, region)
	if err != nil {
		return nil, err
	}
	var plan []RestopItem
	for _, i := range list {
		at := parseTime(i.Tags[StoppedAtTag])
		if !i.Managed() || at.IsZero() || i.Status != "available" || c.now().Sub(at) < minAge {
			continue
		}
		plan = append(plan, RestopItem{ID: i.ID, StoppedAt: at})
	}
	return plan, nil
}

// Restop stops each planned instance again.
func (c *Client) Restop(ctx context.Context, region string, items []RestopItem) error {
	for _, it := range items {
		if err := c.Stop(ctx, region, it.ID, ""); err != nil {
			return err
		}
	}
	return nil
}

// ModifyInput changes class or storage of a managed instance.
type ModifyInput struct {
	Account, Region, ID string
	Class               string
	AllocatedGB         int
	ApplyImmediately    bool
	EstimatedMonthlyUSD float64
}

// ModifyInstance resizes a managed instance. The guard sees the instance tags,
// so the class allowlist and caps apply to the new class.
func (c *Client) ModifyInstance(ctx context.Context, in ModifyInput) error {
	inst, err := c.requireManaged(ctx, in.Region, in.ID)
	if err != nil {
		return err
	}
	if in.Class == "" && in.AllocatedGB == 0 {
		return errors.New("rds: nothing to change")
	}
	if in.AllocatedGB != 0 && in.AllocatedGB <= inst.AllocatedGB {
		return errors.New("rds: storage can only grow")
	}
	d := c.guard.Check(ctx, cloudguard.CreateRequest{Account: in.Account, Service: ServiceName, Resource: in.ID,
		InstanceType: in.Class, EstimatedMonthlyUSD: in.EstimatedMonthlyUSD, Tags: inst.Tags})
	if !d.Allowed {
		return refusal(d)
	}
	v := url.Values{"DBInstanceIdentifier": {in.ID}}
	if in.Class != "" {
		v.Set("DBInstanceClass", in.Class)
	}
	if in.AllocatedGB != 0 {
		v.Set("AllocatedStorage", strconv.Itoa(in.AllocatedGB))
	}
	v.Set("ApplyImmediately", strconv.FormatBool(in.ApplyImmediately))
	return c.aws.Query(ctx, Service, in.Region, "ModifyDBInstance", version, v, nil)
}

// DeleteInput is the delete request. Root and a final snapshot are both mandatory.
type DeleteInput struct {
	Region, ID      string
	Root            bool
	FinalSnapshotID string
}

// DeleteInstance deletes a managed instance after a final snapshot. It never skips the snapshot.
func (c *Client) DeleteInstance(ctx context.Context, in DeleteInput) error {
	if !in.Root || strings.TrimSpace(in.FinalSnapshotID) == "" {
		return ErrNeedsRoot
	}
	if _, err := c.requireManaged(ctx, in.Region, in.ID); err != nil {
		return err
	}
	v := url.Values{"DBInstanceIdentifier": {in.ID}, "SkipFinalSnapshot": {"false"}, "FinalDBSnapshotIdentifier": {in.FinalSnapshotID}}
	return c.aws.Query(ctx, Service, in.Region, "DeleteDBInstance", version, v, nil)
}

// ParameterGroup is the read view of one parameter group.
type ParameterGroup struct{ Name, Family, Description string }

// DescribeDBParameterGroups lists parameter groups (all pages).
func (c *Client) DescribeDBParameterGroups(ctx context.Context, region string) ([]ParameterGroup, error) {
	var res []ParameterGroup
	marker := ""
	for {
		v := url.Values{}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				Name   string `xml:"DBParameterGroupName"`
				Family string `xml:"DBParameterGroupFamily"`
				Desc   string `xml:"Description"`
			} `xml:"DescribeDBParameterGroupsResult>DBParameterGroups>DBParameterGroup"`
			Marker string `xml:"DescribeDBParameterGroupsResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeDBParameterGroups", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, ParameterGroup{Name: x.Name, Family: x.Family, Description: x.Desc})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

// CreateInput describes one instance or cluster create.
type CreateInput struct {
	Account, Region, ID string
	Engine, Class       string
	MasterUsername      string
	DBName              string
	AllocatedGB         int
	MultiAZ             bool
	// ClusterID adds an Aurora member to an existing cluster.
	ClusterID        string
	SubnetGroup      string
	SecurityGroupIDs []string
	BackupRetention  int
	// Confirm is the explicit flag Multi-AZ and Aurora require.
	Confirm             bool
	Meta                Meta
	EstimatedMonthlyUSD float64
}

// IsAurora reports an Aurora engine name.
func IsAurora(engine string) bool { return strings.HasPrefix(engine, "aurora") }

func refusal(d cloudguard.Decision) error { return errors.Join(ErrRefused, errors.New(d.Message)) }

func (c *Client) gate(ctx context.Context, in CreateInput, costly bool) (map[string]string, error) {
	if costly && !in.Confirm {
		return nil, ErrNeedsConfirm
	}
	tags := c.guard.Tags(in.Meta)
	if tags[ExpiresTag] == "" {
		return nil, ErrNoTimer
	}
	tags[tagPrefix+"idle_stop"] = "true"
	d := c.guard.Check(ctx, cloudguard.CreateRequest{Account: in.Account, Service: ServiceName, Resource: in.ID,
		InstanceType: in.Class, EstimatedMonthlyUSD: in.EstimatedMonthlyUSD, Tags: tags})
	if !d.Allowed {
		return nil, refusal(d)
	}
	return tags, nil
}

func sgParams(v url.Values, ids []string) {
	for i, id := range ids {
		v.Set("VpcSecurityGroupIds.VpcSecurityGroupId."+strconv.Itoa(i+1), id)
	}
}

// CreateInstance creates an instance, or an Aurora member when ClusterID is set.
// The master password is generated by RDS and kept in Secrets Manager.
func (c *Client) CreateInstance(ctx context.Context, in CreateInput) error {
	if in.ID == "" || in.Engine == "" || in.Class == "" {
		return errors.New("rds: id, engine and class are required")
	}
	tags, err := c.gate(ctx, in, in.MultiAZ || IsAurora(in.Engine) || in.ClusterID != "")
	if err != nil {
		return err
	}
	v := url.Values{"DBInstanceIdentifier": {in.ID}, "DBInstanceClass": {in.Class}, "Engine": {in.Engine}, "PubliclyAccessible": {"false"}}
	if in.ClusterID != "" {
		v.Set("DBClusterIdentifier", in.ClusterID)
	} else {
		if in.MasterUsername == "" || in.AllocatedGB < 1 {
			return errors.New("rds: master username and allocated storage are required")
		}
		v.Set("MasterUsername", in.MasterUsername)
		v.Set("AllocatedStorage", strconv.Itoa(in.AllocatedGB))
		v.Set("ManageMasterUserPassword", "true")
		v.Set("StorageEncrypted", "true")
		v.Set("MultiAZ", strconv.FormatBool(in.MultiAZ))
		if in.BackupRetention > 0 {
			v.Set("BackupRetentionPeriod", strconv.Itoa(in.BackupRetention))
		}
		if in.DBName != "" {
			v.Set("DBName", in.DBName)
		}
		if in.SubnetGroup != "" {
			v.Set("DBSubnetGroupName", in.SubnetGroup)
		}
		sgParams(v, in.SecurityGroupIDs)
	}
	addTags(v, tags)
	return c.aws.Query(ctx, Service, in.Region, "CreateDBInstance", version, v, nil)
}

// CreateCluster creates an Aurora cluster with no members; add one with CreateInstance and ClusterID.
func (c *Client) CreateCluster(ctx context.Context, in CreateInput) error {
	if in.ID == "" || !IsAurora(in.Engine) || in.MasterUsername == "" {
		return errors.New("rds: id, an Aurora engine and a master username are required")
	}
	tags, err := c.gate(ctx, in, true)
	if err != nil {
		return err
	}
	v := url.Values{"DBClusterIdentifier": {in.ID}, "Engine": {in.Engine}, "MasterUsername": {in.MasterUsername},
		"ManageMasterUserPassword": {"true"}, "StorageEncrypted": {"true"}}
	if in.DBName != "" {
		v.Set("DatabaseName", in.DBName)
	}
	if in.SubnetGroup != "" {
		v.Set("DBSubnetGroupName", in.SubnetGroup)
	}
	if in.BackupRetention > 0 {
		v.Set("BackupRetentionPeriod", strconv.Itoa(in.BackupRetention))
	}
	sgParams(v, in.SecurityGroupIDs)
	addTags(v, tags)
	return c.aws.Query(ctx, Service, in.Region, "CreateDBCluster", version, v, nil)
}

// MaxStopDays is how long AWS keeps a cluster stopped before it starts it again.
const MaxStopDays = 7

// requireManagedCluster loads the cluster and refuses unless it carries the ownership tag.
func (c *Client) requireManagedCluster(ctx context.Context, region, id string) (Cluster, error) {
	list, err := c.DescribeDBClusters(ctx, region, id)
	if err != nil {
		return Cluster{}, err
	}
	if len(list) == 0 {
		return Cluster{}, &awsx.Error{Service: "rds", Op: "DescribeDBClusters", Code: "DBClusterNotFoundFault", Status: 404, Class: awsx.ClassNotFound}
	}
	if !list[0].Managed() {
		return Cluster{}, ErrNotManaged
	}
	return list[0], nil
}

// StopCluster stops a managed cluster and records when; AWS restarts it after MaxStopDays.
func (c *Client) StopCluster(ctx context.Context, region, id string) error {
	k, err := c.requireManagedCluster(ctx, region, id)
	if err != nil {
		return err
	}
	if err := c.aws.Query(ctx, Service, region, "StopDBCluster", version, url.Values{"DBClusterIdentifier": {id}}, nil); err != nil {
		return err
	}
	return c.AddTags(ctx, region, k.ARN, map[string]string{StoppedAtTag: c.now().UTC().Format(time.RFC3339)})
}

// StartCluster starts a managed cluster and clears the stopped marker.
func (c *Client) StartCluster(ctx context.Context, region, id string) error {
	k, err := c.requireManagedCluster(ctx, region, id)
	if err != nil {
		return err
	}
	if err := c.aws.Query(ctx, Service, region, "StartDBCluster", version, url.Values{"DBClusterIdentifier": {id}}, nil); err != nil {
		return err
	}
	return c.AddTags(ctx, region, k.ARN, map[string]string{StoppedAtTag: ""})
}

// ClusterSnapshot is the read view of one cluster snapshot.
type ClusterSnapshot struct {
	ID, ARN, ClusterID, Engine, Status, Type string
	AllocatedGB                              int
	CreatedAt                                time.Time
	Tags                                     map[string]string
}

// Managed reports the ownership tag.
func (s ClusterSnapshot) Managed() bool { return s.Tags[ManagedTag] == "true" }

// DescribeDBClusterSnapshots lists cluster snapshots (all pages), optionally for one cluster or one snapshot.
func (c *Client) DescribeDBClusterSnapshots(ctx context.Context, region, clusterID, snapshotID string) ([]ClusterSnapshot, error) {
	var res []ClusterSnapshot
	marker := ""
	for {
		v := url.Values{}
		if clusterID != "" {
			v.Set("DBClusterIdentifier", clusterID)
		}
		if snapshotID != "" {
			v.Set("DBClusterSnapshotIdentifier", snapshotID)
		}
		if marker != "" {
			v.Set("Marker", marker)
		}
		var out struct {
			Items []struct {
				ID      string `xml:"DBClusterSnapshotIdentifier"`
				ARN     string `xml:"DBClusterSnapshotArn"`
				Cluster string `xml:"DBClusterIdentifier"`
				Engine  string `xml:"Engine"`
				Status  string `xml:"Status"`
				Type    string `xml:"SnapshotType"`
				Alloc   int    `xml:"AllocatedStorage"`
				Created string `xml:"SnapshotCreateTime"`
				Tags    []xTag `xml:"TagList>Tag"`
			} `xml:"DescribeDBClusterSnapshotsResult>DBClusterSnapshots>DBClusterSnapshot"`
			Marker string `xml:"DescribeDBClusterSnapshotsResult>Marker"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeDBClusterSnapshots", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, ClusterSnapshot{ID: x.ID, ARN: x.ARN, ClusterID: x.Cluster, Engine: x.Engine, Status: x.Status, Type: x.Type,
				AllocatedGB: x.Alloc, CreatedAt: parseTime(x.Created), Tags: tagMap(x.Tags)})
		}
		if out.Marker == "" {
			return res, nil
		}
		marker = out.Marker
	}
}

// CreateClusterSnapshot snapshots a managed cluster and tags the snapshot as managed.
func (c *Client) CreateClusterSnapshot(ctx context.Context, region, clusterID, snapshotID string, m Meta) error {
	if _, err := c.requireManagedCluster(ctx, region, clusterID); err != nil {
		return err
	}
	v := url.Values{"DBClusterIdentifier": {clusterID}, "DBClusterSnapshotIdentifier": {snapshotID}}
	addTags(v, c.guard.Tags(m))
	return c.aws.Query(ctx, Service, region, "CreateDBClusterSnapshot", version, v, nil)
}

// DeleteClusterSnapshot removes a cluster snapshot only when it carries the managed tag.
func (c *Client) DeleteClusterSnapshot(ctx context.Context, region, snapshotID string) error {
	list, err := c.DescribeDBClusterSnapshots(ctx, region, "", snapshotID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return &awsx.Error{Service: "rds", Op: "DescribeDBClusterSnapshots", Code: "DBClusterSnapshotNotFoundFault", Status: 404, Class: awsx.ClassNotFound}
	}
	if !list[0].Managed() {
		return ErrNotManaged
	}
	return c.aws.Query(ctx, Service, region, "DeleteDBClusterSnapshot", version, url.Values{"DBClusterSnapshotIdentifier": {snapshotID}}, nil)
}
