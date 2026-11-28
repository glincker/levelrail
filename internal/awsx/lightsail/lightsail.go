// Package lightsail talks to Amazon Lightsail over the signed JSON 1.1 API
// (target prefix Lightsail_20161128). Reads are open; creates go through the
// guard and changes need the managed tag.
package lightsail

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx"
	"github.com/glincker/levelrail/internal/cloudguard"
	"github.com/glincker/levelrail/internal/cloudspend"
)

// Service is the Lightsail JSON API.
var Service = awsx.Service{Name: "lightsail", Prefix: "lightsail", SigningName: "lightsail", TargetPrefix: "Lightsail_20161128"}

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "lightsail"

// Errors the API layer maps to HTTP statuses.
var (
	ErrRefused      = errors.New("lightsail: refused")
	ErrNotManaged   = errors.New("lightsail: resource is not managed by this control plane")
	ErrNeedsConfirm = errors.New("lightsail: delete needs the resource name typed as confirmation")
	ErrNoTimer      = errors.New("lightsail: a teardown timer (expires tag) is required")
)

var tagPrefix = strings.TrimSuffix(cloudspend.ProjectTagKey, "project")

// ManagedTag marks resources this control plane owns.
var ManagedTag = tagPrefix + "managed"

// Guard is the create gate and tag source.
type Guard interface {
	Check(ctx context.Context, r cloudguard.CreateRequest) cloudguard.Decision
	Tags(m cloudguard.TagMeta) map[string]string
}

// Meta carries the values behind the mandatory tags.
type Meta = cloudguard.TagMeta

// Client is the lazy Lightsail module; it makes no call until a method runs.
type Client struct {
	aws   *awsx.Client
	guard Guard
}

// New wraps a shared client and guard.
func New(c *awsx.Client, g Guard) *Client { return &Client{aws: c, guard: g} }

func notFound(op, code string) error {
	return &awsx.Error{Service: ServiceName, Op: op, Code: code, Status: 404, Class: awsx.ClassNotFound}
}

func refusal(msg string) error { return errors.Join(ErrRefused, errors.New(msg)) }

type jTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func tagMap(l []jTag) map[string]string {
	m := make(map[string]string, len(l))
	for _, t := range l {
		m[t.Key] = t.Value
	}
	return m
}

// tagParams converts a tag map to the lowercase key and value shape Lightsail uses.
func tagParams(m map[string]string) []jTag {
	list := awsx.TagList(m)
	out := make([]jTag, 0, len(list))
	for _, t := range list {
		out = append(out, jTag{Key: t.Key, Value: t.Value})
	}
	return out
}

// Instance is the read view of one instance.
type Instance struct {
	Name, ARN, State, BlueprintID, BundleID string
	PublicIP, PrivateIP, Zone, Region       string
	CPUs                                    int
	RAMGB, DiskGB                           float64
	CreatedAt                               time.Time
	Managed                                 bool
	Tags                                    map[string]string
}

type jInstance struct {
	Name        string         `json:"name"`
	ARN         string         `json:"arn"`
	CreatedAt   awsx.EpochTime `json:"createdAt"`
	BlueprintID string         `json:"blueprintId"`
	BundleID    string         `json:"bundleId"`
	PublicIP    string         `json:"publicIpAddress"`
	PrivateIP   string         `json:"privateIpAddress"`
	Location    struct {
		Zone   string `json:"availabilityZone"`
		Region string `json:"regionName"`
	} `json:"location"`
	Hardware struct {
		CPUs  int     `json:"cpuCount"`
		RAMGB float64 `json:"ramSizeInGb"`
		Disks []struct {
			SizeInGB float64 `json:"sizeInGb"`
		} `json:"disks"`
	} `json:"hardware"`
	State struct {
		Name string `json:"name"`
	} `json:"state"`
	Tags []jTag `json:"tags"`
}

func (j jInstance) toInstance() Instance {
	tags := tagMap(j.Tags)
	var disk float64
	for _, d := range j.Hardware.Disks {
		disk += d.SizeInGB
	}
	return Instance{Name: j.Name, ARN: j.ARN, State: j.State.Name, BlueprintID: j.BlueprintID, BundleID: j.BundleID,
		PublicIP: j.PublicIP, PrivateIP: j.PrivateIP, Zone: j.Location.Zone, Region: j.Location.Region,
		CPUs: j.Hardware.CPUs, RAMGB: j.Hardware.RAMGB, DiskGB: disk, CreatedAt: j.CreatedAt.UTC(),
		Managed: tags[ManagedTag] == "true", Tags: tags}
}

// GetInstances lists instances (all pages) in a region.
func (c *Client) GetInstances(ctx context.Context, region string) ([]Instance, error) {
	var res []Instance
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["pageToken"] = token
		}
		var out struct {
			Items []jInstance `json:"instances"`
			Next  string      `json:"nextPageToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "GetInstances", in, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, x.toInstance())
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// GetInstance reads one instance by name.
func (c *Client) GetInstance(ctx context.Context, region, name string) (Instance, error) {
	var out struct {
		Instance *jInstance `json:"instance"`
	}
	if err := c.aws.JSON(ctx, Service, region, "GetInstance", map[string]any{"instanceName": name}, &out); err != nil {
		return Instance{}, err
	}
	if out.Instance == nil {
		return Instance{}, notFound("GetInstance", "NotFoundException")
	}
	return out.Instance.toInstance(), nil
}

// Bundle is one size and price plan. PriceUSD comes from AWS, never from a table here.
type Bundle struct {
	ID, Name           string
	PriceUSD           float64
	CPUs               int
	RAMGB, DiskGB      float64
	TransferGB         int
	Active             bool
	SupportedPlatforms []string
}

// GetBundles lists bundles (all pages).
func (c *Client) GetBundles(ctx context.Context, region string) ([]Bundle, error) {
	var res []Bundle
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["pageToken"] = token
		}
		var out struct {
			Items []struct {
				ID        string   `json:"bundleId"`
				Name      string   `json:"name"`
				Price     float64  `json:"price"`
				CPUs      int      `json:"cpuCount"`
				RAMGB     float64  `json:"ramSizeInGb"`
				DiskGB    float64  `json:"diskSizeInGb"`
				Transfer  int      `json:"transferPerMonthInGb"`
				Active    bool     `json:"isActive"`
				Platforms []string `json:"supportedPlatforms"`
			} `json:"bundles"`
			Next string `json:"nextPageToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "GetBundles", in, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Bundle{ID: x.ID, Name: x.Name, PriceUSD: x.Price, CPUs: x.CPUs, RAMGB: x.RAMGB, DiskGB: x.DiskGB,
				TransferGB: x.Transfer, Active: x.Active, SupportedPlatforms: x.Platforms})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// Blueprint is one OS or app image.
type Blueprint struct {
	ID, Name, Group, Type, Version, Platform string
	Active                                   bool
}

// GetBlueprints lists blueprints (all pages).
func (c *Client) GetBlueprints(ctx context.Context, region string) ([]Blueprint, error) {
	var res []Blueprint
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["pageToken"] = token
		}
		var out struct {
			Items []struct {
				ID       string `json:"blueprintId"`
				Name     string `json:"name"`
				Group    string `json:"group"`
				Type     string `json:"type"`
				Version  string `json:"version"`
				Platform string `json:"platform"`
				Active   bool   `json:"isActive"`
			} `json:"blueprints"`
			Next string `json:"nextPageToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "GetBlueprints", in, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, Blueprint{ID: x.ID, Name: x.Name, Group: x.Group, Type: x.Type, Version: x.Version, Platform: x.Platform, Active: x.Active})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// Region is one Lightsail region with its zones.
type Region struct {
	Name, DisplayName string
	Zones             []string
}

// GetRegions lists Lightsail regions with their availability zones.
func (c *Client) GetRegions(ctx context.Context, region string) ([]Region, error) {
	var out struct {
		Regions []struct {
			Name    string `json:"name"`
			Display string `json:"displayName"`
			Zones   []struct {
				Name string `json:"zoneName"`
			} `json:"availabilityZones"`
		} `json:"regions"`
	}
	if err := c.aws.JSON(ctx, Service, region, "GetRegions", map[string]any{"includeAvailabilityZones": true}, &out); err != nil {
		return nil, err
	}
	res := make([]Region, 0, len(out.Regions))
	for _, x := range out.Regions {
		r := Region{Name: x.Name, DisplayName: x.Display}
		for _, z := range x.Zones {
			r.Zones = append(r.Zones, z.Name)
		}
		res = append(res, r)
	}
	return res, nil
}

// StaticIP is one static address.
type StaticIP struct {
	Name, IP, AttachedTo string
	Attached             bool
}

// GetStaticIps lists static addresses (all pages).
func (c *Client) GetStaticIps(ctx context.Context, region string) ([]StaticIP, error) {
	var res []StaticIP
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["pageToken"] = token
		}
		var out struct {
			Items []struct {
				Name     string `json:"name"`
				IP       string `json:"ipAddress"`
				To       string `json:"attachedTo"`
				Attached bool   `json:"isAttached"`
			} `json:"staticIps"`
			Next string `json:"nextPageToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "GetStaticIps", in, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, StaticIP{Name: x.Name, IP: x.IP, AttachedTo: x.To, Attached: x.Attached})
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

// Snapshot is the read view of one instance snapshot.
type Snapshot struct {
	Name, ARN, State, FromInstance, FromBundle string
	SizeGB                                     int
	CreatedAt                                  time.Time
	Managed                                    bool
}

// GetInstanceSnapshots lists instance snapshots (all pages).
func (c *Client) GetInstanceSnapshots(ctx context.Context, region string) ([]Snapshot, error) {
	var res []Snapshot
	token := ""
	for {
		in := map[string]any{}
		if token != "" {
			in["pageToken"] = token
		}
		var out struct {
			Items []jSnapshot `json:"instanceSnapshots"`
			Next  string      `json:"nextPageToken"`
		}
		if err := c.aws.JSON(ctx, Service, region, "GetInstanceSnapshots", in, &out); err != nil {
			return nil, err
		}
		for _, x := range out.Items {
			res = append(res, x.toSnapshot())
		}
		if out.Next == "" {
			return res, nil
		}
		token = out.Next
	}
}

type jSnapshot struct {
	Name         string         `json:"name"`
	ARN          string         `json:"arn"`
	State        string         `json:"state"`
	FromInstance string         `json:"fromInstanceName"`
	FromBundle   string         `json:"fromBundleId"`
	SizeGB       int            `json:"sizeInGb"`
	Tags         []jTag         `json:"tags"`
	Created      awsx.EpochTime `json:"createdAt"`
}

func (j jSnapshot) toSnapshot() Snapshot {
	return Snapshot{Name: j.Name, ARN: j.ARN, State: j.State, FromInstance: j.FromInstance, FromBundle: j.FromBundle,
		SizeGB: j.SizeGB, CreatedAt: j.Created.UTC(), Managed: tagMap(j.Tags)[ManagedTag] == "true"}
}

func (c *Client) requireManaged(ctx context.Context, region, name string) error {
	inst, err := c.GetInstance(ctx, region, name)
	if err != nil {
		return err
	}
	if !inst.Managed {
		return ErrNotManaged
	}
	return nil
}

// Start starts a managed instance.
func (c *Client) Start(ctx context.Context, region, name string) error {
	return c.power(ctx, region, name, "StartInstance")
}

// Stop stops a managed instance.
func (c *Client) Stop(ctx context.Context, region, name string) error {
	return c.power(ctx, region, name, "StopInstance")
}

// Reboot reboots a managed instance.
func (c *Client) Reboot(ctx context.Context, region, name string) error {
	return c.power(ctx, region, name, "RebootInstance")
}

func (c *Client) power(ctx context.Context, region, name, op string) error {
	if err := c.requireManaged(ctx, region, name); err != nil {
		return err
	}
	return c.aws.JSON(ctx, Service, region, op, map[string]any{"instanceName": name}, nil)
}

// DeleteInput is a delete request. Confirm must repeat the instance name.
type DeleteInput struct{ Region, Name, Confirm string }

// Delete removes a managed instance after a typed confirm. Add-ons are not force deleted.
func (c *Client) Delete(ctx context.Context, in DeleteInput) error {
	if in.Name == "" || in.Confirm != in.Name {
		return ErrNeedsConfirm
	}
	if err := c.requireManaged(ctx, in.Region, in.Name); err != nil {
		return err
	}
	return c.aws.JSON(ctx, Service, in.Region, "DeleteInstance", map[string]any{"instanceName": in.Name}, nil)
}

// CreateSnapshot snapshots a managed instance and tags the snapshot as managed.
func (c *Client) CreateSnapshot(ctx context.Context, region, instance, name string, m Meta) error {
	if err := c.requireManaged(ctx, region, instance); err != nil {
		return err
	}
	in := map[string]any{"instanceSnapshotName": name, "instanceName": instance, "tags": tagParams(c.guard.Tags(m))}
	return c.aws.JSON(ctx, Service, region, "CreateInstanceSnapshot", in, nil)
}

// DeleteSnapshot deletes a snapshot this control plane created.
func (c *Client) DeleteSnapshot(ctx context.Context, region, name string) error {
	list, err := c.GetInstanceSnapshots(ctx, region)
	if err != nil {
		return err
	}
	for _, s := range list {
		if s.Name != name {
			continue
		}
		if !s.Managed {
			return ErrNotManaged
		}
		return c.aws.JSON(ctx, Service, region, "DeleteInstanceSnapshot", map[string]any{"instanceSnapshotName": name}, nil)
	}
	return notFound("GetInstanceSnapshots", "NotFoundException")
}

// CreateInput describes one instance create. AllowedBundles and MaxManaged come from runtime settings.
type CreateInput struct {
	Account, Region, Name, Zone string
	Blueprint, Bundle, KeyPair  string
	Meta                        Meta
	AllowedBundles              []string
	// MaxManaged is the most managed instances the account may hold in the region; 0 refuses every create.
	MaxManaged int
}

// Create makes one managed instance after the guard, bundle allowlist and instance cap pass.
// The price is read from GetBundles, so the monthly cap and per create limit see the real figure.
func (c *Client) Create(ctx context.Context, in CreateInput) error {
	if in.Name == "" || in.Zone == "" || in.Blueprint == "" || in.Bundle == "" {
		return errors.New("lightsail: name, availability zone, blueprint and bundle are required")
	}
	if !containsStr(in.AllowedBundles, in.Bundle) {
		return refusal("This bundle is not on the allowlist.")
	}
	bundles, err := c.GetBundles(ctx, in.Region)
	if err != nil {
		return err
	}
	price := -1.0
	for _, b := range bundles {
		if b.ID == in.Bundle && b.Active {
			price = b.PriceUSD
		}
	}
	if price < 0 {
		return refusal("This bundle is not offered in the region.")
	}
	tags := c.guard.Tags(in.Meta)
	if tags[tagPrefix+"expires"] == "" {
		return ErrNoTimer
	}
	d := c.guard.Check(ctx, cloudguard.CreateRequest{Account: in.Account, Service: ServiceName, Resource: in.Name,
		EstimatedMonthlyUSD: price, Tags: tags})
	if !d.Allowed {
		return refusal(d.Message)
	}
	existing, err := c.GetInstances(ctx, in.Region)
	if err != nil {
		return err
	}
	n := 0
	for _, i := range existing {
		if i.Managed {
			n++
		}
	}
	if n >= in.MaxManaged {
		return refusal("The managed instance limit for this region is reached.")
	}
	body := map[string]any{"instanceNames": []string{in.Name}, "availabilityZone": in.Zone, "blueprintId": in.Blueprint,
		"bundleId": in.Bundle, "tags": tagParams(tags)}
	if in.KeyPair != "" {
		body["keyPairName"] = in.KeyPair
	}
	return c.aws.JSON(ctx, Service, in.Region, "CreateInstances", body, nil)
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
