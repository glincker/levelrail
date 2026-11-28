package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx/lightsail"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

type lightsailInstanceWire struct {
	Name        string    `json:"name"`
	State       string    `json:"state"`
	BlueprintID string    `json:"blueprint_id"`
	BundleID    string    `json:"bundle_id"`
	PublicIP    string    `json:"public_ip,omitempty"`
	PrivateIP   string    `json:"private_ip,omitempty"`
	Zone        string    `json:"zone"`
	CPUs        int       `json:"cpus"`
	RAMGB       float64   `json:"ram_gb"`
	DiskGB      float64   `json:"disk_gb"`
	CreatedAt   time.Time `json:"created_at"`
	Managed     bool      `json:"managed"`
}

func toLightsailInstanceWire(i lightsail.Instance) lightsailInstanceWire {
	return lightsailInstanceWire{Name: i.Name, State: i.State, BlueprintID: i.BlueprintID, BundleID: i.BundleID, PublicIP: i.PublicIP,
		PrivateIP: i.PrivateIP, Zone: i.Zone, CPUs: i.CPUs, RAMGB: i.RAMGB, DiskGB: i.DiskGB, CreatedAt: i.CreatedAt, Managed: i.Managed}
}

func writeLightsailError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lightsail.ErrNotManaged):
		writeError(w, http.StatusConflict, "this instance is not managed here")
	case errors.Is(err, lightsail.ErrNeedsConfirm):
		writeError(w, http.StatusBadRequest, "confirm must repeat the resource name")
	case errors.Is(err, lightsail.ErrRefused), errors.Is(err, lightsail.ErrNoTimer):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeAWSError(w, err)
	}
}

func (rt *Router) handleLightsailInstances(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetInstances(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]lightsailInstanceWire, 0, len(list))
	for _, i := range list {
		out = append(out, toLightsailInstanceWire(i))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "instances": out})
}

func (rt *Router) handleLightsailInstance(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	i, err := lightsail.New(c.client, c.guard).GetInstance(r.Context(), c.region, r.PathValue("name"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toLightsailInstanceWire(i))
}

func (rt *Router) handleLightsailBundles(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetBundles(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	allowed := map[string]bool{}
	for _, b := range rt.settingList(runtimesettings.KeyCloudGuardLSBundles) {
		allowed[b] = true
	}
	type bundleWire struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		PriceUSD   float64 `json:"price_usd"`
		CPUs       int     `json:"cpus"`
		RAMGB      float64 `json:"ram_gb"`
		DiskGB     float64 `json:"disk_gb"`
		TransferGB int     `json:"transfer_gb"`
		Allowed    bool    `json:"allowed"`
	}
	out := make([]bundleWire, 0, len(list))
	for _, b := range list {
		if b.Active {
			out = append(out, bundleWire{ID: b.ID, Name: b.Name, PriceUSD: b.PriceUSD, CPUs: b.CPUs, RAMGB: b.RAMGB, DiskGB: b.DiskGB, TransferGB: b.TransferGB, Allowed: allowed[b.ID]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "bundles": out})
}

func (rt *Router) handleLightsailBlueprints(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetBlueprints(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type bpWire struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Group    string `json:"group"`
		Type     string `json:"type"`
		Version  string `json:"version"`
		Platform string `json:"platform"`
	}
	out := make([]bpWire, 0, len(list))
	for _, b := range list {
		if b.Active {
			out = append(out, bpWire{ID: b.ID, Name: b.Name, Group: b.Group, Type: b.Type, Version: b.Version, Platform: b.Platform})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "blueprints": out})
}

func (rt *Router) handleLightsailRegions(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetRegions(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type regionWire struct {
		Name  string   `json:"name"`
		Label string   `json:"display_name"`
		Zones []string `json:"zones"`
	}
	out := make([]regionWire, 0, len(list))
	for _, g := range list {
		z := g.Zones
		if z == nil {
			z = []string{}
		}
		out = append(out, regionWire{Name: g.Name, Label: g.DisplayName, Zones: z})
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": out})
}

func (rt *Router) handleLightsailStaticIPs(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetStaticIps(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type ipWire struct {
		Name       string `json:"name"`
		IP         string `json:"ip"`
		AttachedTo string `json:"attached_to,omitempty"`
		Attached   bool   `json:"attached"`
	}
	out := make([]ipWire, 0, len(list))
	for _, s := range list {
		out = append(out, ipWire{Name: s.Name, IP: s.IP, AttachedTo: s.AttachedTo, Attached: s.Attached})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "static_ips": out})
}

func (rt *Router) lightsailPower(w http.ResponseWriter, r *http.Request, action string) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	name := r.PathValue("name")
	cl := lightsail.New(c.client, c.guard)
	var err error
	switch action {
	case "start":
		err = cl.Start(r.Context(), c.region, name)
	case "stop":
		err = cl.Stop(r.Context(), c.region, name)
	default:
		err = cl.Reboot(r.Context(), c.region, name)
	}
	if err != nil {
		rt.auditCloud(r, "cloud.aws.lightsail."+action+"_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
		writeLightsailError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.lightsail."+action, map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": name, "action": action})
}

func (rt *Router) handleLightsailStart(w http.ResponseWriter, r *http.Request) {
	rt.lightsailPower(w, r, "start")
}

func (rt *Router) handleLightsailStop(w http.ResponseWriter, r *http.Request) {
	rt.lightsailPower(w, r, "stop")
}

func (rt *Router) handleLightsailReboot(w http.ResponseWriter, r *http.Request) {
	rt.lightsailPower(w, r, "reboot")
}

type lightsailCreateBody struct {
	Region    string `json:"region"`
	Name      string `json:"name"`
	Zone      string `json:"availability_zone"`
	Blueprint string `json:"blueprint_id"`
	Bundle    string `json:"bundle_id"`
	KeyPair   string `json:"key_pair"`
	Env       string `json:"env"`
	Project   string `json:"project"`
}

func (rt *Router) lightsailMaxInstances() int {
	if rt.runtimeSettings == nil {
		return 0
	}
	return rt.runtimeSettings.Int(context.Background(), runtimesettings.KeyCloudGuardLSMax)
}

func (rt *Router) handleLightsailCreate(w http.ResponseWriter, r *http.Request) {
	var body lightsailCreateBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.Name) == "" || body.Bundle == "" || body.Blueprint == "" || body.Zone == "" {
		writeError(w, http.StatusBadRequest, "a name, availability_zone, blueprint_id and bundle_id are required")
		return
	}
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, body.Region)
	if !ok {
		return
	}
	in := lightsail.CreateInput{Account: c.acct.ID, Region: c.region, Name: body.Name, Zone: body.Zone, Blueprint: body.Blueprint,
		Bundle: body.Bundle, KeyPair: body.KeyPair, Meta: rt.awsTagMeta(r, body.Name, body.Env, body.Project),
		AllowedBundles: rt.settingList(runtimesettings.KeyCloudGuardLSBundles), MaxManaged: rt.lightsailMaxInstances()}
	if err := lightsail.New(c.client, c.guard).Create(r.Context(), in); err != nil {
		rt.auditCloud(r, "cloud.aws.lightsail.create_refused", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.Name})
		writeLightsailError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.lightsail.create", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.Name, "bundle": body.Bundle})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": body.Name, "action": "create"})
}

// handleLightsailDelete is blocked by the kill switch like any guarded change.
func (rt *Router) handleLightsailDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	if rt.cloudGuardConfig().KillSwitch {
		writeError(w, http.StatusConflict, "the cloud guard kill switch is on")
		return
	}
	name := r.PathValue("name")
	err := lightsail.New(c.client, c.guard).Delete(r.Context(), lightsail.DeleteInput{Region: c.region, Name: name, Confirm: r.URL.Query().Get("confirm")})
	if err != nil {
		rt.auditCloud(r, "cloud.aws.lightsail.delete_refused", map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
		writeLightsailError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.lightsail.delete", map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": name, "action": "delete"})
}

func (rt *Router) handleLightsailSnapshots(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	list, err := lightsail.New(c.client, c.guard).GetInstanceSnapshots(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type snapWire struct {
		Name      string    `json:"name"`
		State     string    `json:"state"`
		Instance  string    `json:"instance,omitempty"`
		SizeGB    int       `json:"size_gb"`
		CreatedAt time.Time `json:"created_at"`
		Managed   bool      `json:"managed"`
	}
	out := make([]snapWire, 0, len(list))
	for _, s := range list {
		out = append(out, snapWire{Name: s.Name, State: s.State, Instance: s.FromInstance, SizeGB: s.SizeGB, CreatedAt: s.CreatedAt, Managed: s.Managed})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "snapshots": out})
}

type lightsailSnapshotBody struct {
	Region   string `json:"region"`
	Instance string `json:"instance"`
	Name     string `json:"name"`
	Env      string `json:"env"`
	Project  string `json:"project"`
}

func (rt *Router) handleLightsailSnapshotCreate(w http.ResponseWriter, r *http.Request) {
	var body lightsailSnapshotBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.Instance) == "" {
		writeError(w, http.StatusBadRequest, "an instance is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, body.Region)
	if !ok {
		return
	}
	name := body.Name
	if name == "" {
		name = rdsSnapshotName(body.Instance)
	}
	err := lightsail.New(c.client, c.guard).CreateSnapshot(r.Context(), c.region, body.Instance, name, rt.awsTagMeta(r, body.Instance, body.Env, body.Project))
	if err != nil {
		rt.auditCloud(r, "cloud.aws.lightsail.snapshot_failed", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.Instance})
		writeLightsailError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.lightsail.snapshot", map[string]any{"account": c.acct.ID, "region": c.region, "id": body.Instance, "snapshot": name})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": body.Instance, "snapshot_name": name})
}

func (rt *Router) handleLightsailSnapshotDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, lightsail.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	name := r.PathValue("name")
	if err := lightsail.New(c.client, c.guard).DeleteSnapshot(r.Context(), c.region, name); err != nil {
		rt.auditCloud(r, "cloud.aws.lightsail.snapshot_delete_refused", map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
		writeLightsailError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.lightsail.snapshot_delete", map[string]any{"account": c.acct.ID, "region": c.region, "id": name})
	writeJSON(w, http.StatusAccepted, map[string]any{"id": name, "action": "delete"})
}
