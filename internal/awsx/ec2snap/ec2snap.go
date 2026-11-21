// Package ec2snap manages EBS snapshots over the signed EC2 Query API (version
// 2016-11-15). Deletes only touch snapshots that carry the managed tag.
package ec2snap

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
	"github.com/GLINCKER/levelrail/internal/cloudspend"
)

// Service is the EC2 Query API.
var Service = awsx.Service{Name: "ec2", Prefix: "ec2", SigningName: "ec2"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "ebs"

const (
	version  = "2016-11-15"
	maxPages = 20
)

// Errors the API layer maps to HTTP statuses.
var (
	ErrNotManaged = errors.New("ec2snap: snapshot is not managed by this control plane")
	ErrNoSource   = errors.New("ec2snap: an instance or a volume id is required")
)

var tagPrefix = strings.TrimSuffix(cloudspend.ProjectTagKey, "project")

// ManagedTag marks snapshots this control plane created.
var ManagedTag = tagPrefix + "managed"

// ExpiresTag is the teardown timer written by the guard.
var ExpiresTag = tagPrefix + "expires"

// Guard is the tag source.
type Guard interface {
	Tags(m cloudguard.TagMeta) map[string]string
}

// Meta carries the values behind the mandatory tags.
type Meta = cloudguard.TagMeta

// Client is the lazy EBS module; it makes no call until a method runs.
type Client struct {
	aws   *awsx.Client
	guard Guard
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard) *Client { return &Client{aws: c, guard: g} }

type xTag struct {
	Key   string `xml:"key"`
	Value string `xml:"value"`
}

func tagMap(in []xTag) map[string]string {
	m := make(map[string]string, len(in))
	for _, t := range in {
		m[t.Key] = t.Value
	}
	return m
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// Snapshot is the read view of one EBS snapshot.
type Snapshot struct {
	ID, VolumeID, State, Description string
	SizeGB                           int
	StartTime                        time.Time
	Tags                             map[string]string
}

// Managed reports the ownership tag.
func (s Snapshot) Managed() bool { return s.Tags[ManagedTag] == "true" }

// ExpiresAt is the expires tag, zero when absent.
func (s Snapshot) ExpiresAt() time.Time { return parseTime(s.Tags[ExpiresTag]) }

// Volume is the read view of one EBS volume.
type Volume struct {
	ID, Type, State, AZ, InstanceID string
	SizeGB                          int
	Encrypted                       bool
	Tags                            map[string]string
}

// Managed reports the ownership tag.
func (v Volume) Managed() bool { return v.Tags[ManagedTag] == "true" }

type xSnapshot struct {
	ID       string `xml:"snapshotId"`
	VolumeID string `xml:"volumeId"`
	State    string `xml:"status"`
	// CreateSnapshots answers with state instead of status.
	State2 string `xml:"state"`
	Desc   string `xml:"description"`
	Size   int    `xml:"volumeSize"`
	Start  string `xml:"startTime"`
	Tags   []xTag `xml:"tagSet>item"`
}

func (x xSnapshot) toSnapshot() Snapshot {
	st := x.State
	if st == "" {
		st = x.State2
	}
	return Snapshot{ID: x.ID, VolumeID: x.VolumeID, State: st, Description: x.Desc, SizeGB: x.Size, StartTime: parseTime(x.Start), Tags: tagMap(x.Tags)}
}

func addTagSpec(v url.Values, resource string, tags map[string]string) {
	v.Set("TagSpecification.1.ResourceType", resource)
	for i, t := range awsx.TagList(tags) {
		p := "TagSpecification.1.Tag." + strconv.Itoa(i+1)
		v.Set(p+".Key", t.Key)
		v.Set(p+".Value", t.Value)
	}
}

// DescribeSnapshots lists snapshots owned by the account (all pages), optionally for one volume.
func (c *Client) DescribeSnapshots(ctx context.Context, region, volumeID string) ([]Snapshot, error) {
	var res []Snapshot
	token := ""
	for page := 0; page < maxPages; page++ {
		v := url.Values{"Owner.1": {"self"}, "MaxResults": {"500"}}
		if volumeID != "" {
			v.Set("Filter.1.Name", "volume-id")
			v.Set("Filter.1.Value.1", volumeID)
		}
		if token != "" {
			v.Set("NextToken", token)
		}
		var out struct {
			NextToken string      `xml:"nextToken"`
			Items     []xSnapshot `xml:"snapshotSet>item"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeSnapshots", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, x.toSnapshot())
		}
		if out.NextToken == "" {
			break
		}
		token = out.NextToken
	}
	return res, nil
}

// DescribeVolumes lists volumes (all pages), optionally the one named.
func (c *Client) DescribeVolumes(ctx context.Context, region, volumeID string) ([]Volume, error) {
	var res []Volume
	token := ""
	for page := 0; page < maxPages; page++ {
		v := url.Values{}
		if volumeID != "" {
			v.Set("VolumeId.1", volumeID)
		} else {
			v.Set("MaxResults", "500")
		}
		if token != "" {
			v.Set("NextToken", token)
		}
		var out struct {
			NextToken string `xml:"nextToken"`
			Items     []struct {
				ID        string `xml:"volumeId"`
				Size      int    `xml:"size"`
				Type      string `xml:"volumeType"`
				State     string `xml:"status"`
				AZ        string `xml:"availabilityZone"`
				Encrypted bool   `xml:"encrypted"`
				Instance  string `xml:"attachmentSet>item>instanceId"`
				Tags      []xTag `xml:"tagSet>item"`
			} `xml:"volumeSet>item"`
		}
		if err := c.aws.Query(ctx, Service, region, "DescribeVolumes", version, v, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Volume{ID: x.ID, SizeGB: x.Size, Type: x.Type, State: x.State, AZ: x.AZ, Encrypted: x.Encrypted, InstanceID: x.Instance, Tags: tagMap(x.Tags)})
		}
		if out.NextToken == "" || volumeID != "" {
			break
		}
		token = out.NextToken
	}
	return res, nil
}

// CreateInput names what to snapshot: every volume of an instance, or one volume.
type CreateInput struct {
	Region, InstanceID, VolumeID, Description string
	ExcludeBoot                               bool
	Meta                                      Meta
}

// CreateSnapshots takes crash consistent snapshots of all volumes of an instance
// (CreateSnapshots) or of one volume (CreateSnapshot). Every snapshot gets the
// managed tags and the expires tag from the guard.
func (c *Client) CreateSnapshots(ctx context.Context, in CreateInput) ([]Snapshot, error) {
	tags := c.guard.Tags(in.Meta)
	v := url.Values{}
	if in.Description != "" {
		v.Set("Description", in.Description)
	}
	addTagSpec(v, "snapshot", tags)
	switch {
	case in.InstanceID != "":
		v.Set("InstanceSpecification.InstanceId", in.InstanceID)
		if in.ExcludeBoot {
			v.Set("InstanceSpecification.ExcludeBootVolume", "true")
		}
		v.Set("CopyTagsFromSource", "volume")
		var out struct {
			Items []xSnapshot `xml:"snapshotSet>item"`
		}
		if err := c.aws.Query(ctx, Service, in.Region, "CreateSnapshots", version, v, &out); err != nil {
			return nil, err
		}
		res := make([]Snapshot, 0, len(out.Items))
		for _, x := range out.Items {
			res = append(res, x.toSnapshot())
		}
		return res, nil
	case in.VolumeID != "":
		v.Set("VolumeId", in.VolumeID)
		var out xSnapshot
		if err := c.aws.Query(ctx, Service, in.Region, "CreateSnapshot", version, v, &out); err != nil {
			return nil, err
		}
		if out.VolumeID == "" {
			out.VolumeID = in.VolumeID
		}
		return []Snapshot{out.toSnapshot()}, nil
	}
	return nil, ErrNoSource
}

// DeleteSnapshot removes a snapshot only when it carries the managed tag.
func (c *Client) DeleteSnapshot(ctx context.Context, region, snapshotID string) error {
	var out struct {
		Items []xSnapshot `xml:"snapshotSet>item"`
	}
	if err := c.aws.Query(ctx, Service, region, "DescribeSnapshots", version, url.Values{"SnapshotId.1": {snapshotID}}, &out); err != nil {
		return err
	}
	if len(out.Items) == 0 {
		return &awsx.Error{Service: "ec2", Op: "DescribeSnapshots", Code: "InvalidSnapshot.NotFound", Status: 404, Class: awsx.ClassNotFound}
	}
	if !out.Items[0].toSnapshot().Managed() {
		return ErrNotManaged
	}
	return c.aws.Query(ctx, Service, region, "DeleteSnapshot", version, url.Values{"SnapshotId": {snapshotID}}, nil)
}

// Retention is how many daily and weekly snapshots to keep per volume.
type Retention struct{ Daily, Weekly int }

// Deletion is one snapshot the sweeper would remove.
type Deletion struct {
	ID, VolumeID string
	StartTime    time.Time
	SizeGB       int
	Reason       string
}

// ReasonExpired is the cleanup plan item kind for a snapshot past retention.
const ReasonExpired = "snapshot_expired"

// PlanRetention is a dry run: per volume it keeps the newest managed snapshot of
// each of the Daily most recent days and the Weekly most recent weeks, and lists the
// rest. Untagged and unmanaged snapshots are never listed.
func PlanRetention(list []Snapshot, r Retention) []Deletion {
	if r.Daily < 1 && r.Weekly < 1 {
		return nil // no policy means keep everything
	}
	byVol := map[string][]Snapshot{}
	for _, s := range list {
		if s.Managed() && !s.StartTime.IsZero() {
			byVol[s.VolumeID] = append(byVol[s.VolumeID], s)
		}
	}
	var out []Deletion
	for _, snaps := range byVol {
		sort.Slice(snaps, func(i, j int) bool { return snaps[i].StartTime.After(snaps[j].StartTime) })
		keep := map[string]bool{}
		days, weeks := map[string]bool{}, map[string]bool{}
		for _, s := range snaps {
			d := s.StartTime.Format("2006-01-02")
			y, w := s.StartTime.ISOWeek()
			wk := strconv.Itoa(y) + "-" + strconv.Itoa(w)
			if !days[d] && len(days) < r.Daily {
				days[d] = true
				keep[s.ID] = true
			}
			if !weeks[wk] && len(weeks) < r.Weekly {
				weeks[wk] = true
				keep[s.ID] = true
			}
		}
		for _, s := range snaps {
			if !keep[s.ID] {
				out = append(out, Deletion{ID: s.ID, VolumeID: s.VolumeID, StartTime: s.StartTime, SizeGB: s.SizeGB, Reason: ReasonExpired})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// PlanRetentionFor reads the snapshots and plans deletions. It changes nothing.
func (c *Client) PlanRetentionFor(ctx context.Context, region string, r Retention) ([]Deletion, error) {
	list, err := c.DescribeSnapshots(ctx, region, "")
	if err != nil {
		return nil, err
	}
	return PlanRetention(list, r), nil
}
