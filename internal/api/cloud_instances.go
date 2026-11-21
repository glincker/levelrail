package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/cloudspend"
	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// cloudInstanceState is the per-router list cache; routers are few, so a keyed map is enough.
type cloudInstanceState struct {
	mu    sync.Mutex
	lists map[string]cloudInstanceList
}

type cloudInstanceList struct {
	at   time.Time
	rows []cloudInstanceWire
}

var cloudInstanceStates sync.Map

func (rt *Router) cloudInstanceState() *cloudInstanceState {
	v, _ := cloudInstanceStates.LoadOrStore(rt, &cloudInstanceState{lists: map[string]cloudInstanceList{}})
	return v.(*cloudInstanceState)
}

func (s *cloudInstanceState) drop(account string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.lists {
		if strings.HasPrefix(k, account+"|") {
			delete(s.lists, k)
		}
	}
}

type cloudInstanceWire struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Region     string            `json:"region"`
	State      string            `json:"state"`
	RawState   string            `json:"raw_state"`
	Shape      string            `json:"shape"`
	OCPUs      float64           `json:"ocpus,omitempty"`
	MemoryGB   float64           `json:"memory_gb,omitempty"`
	PublicIPv4 string            `json:"public_ipv4,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	Tags       map[string]string `json:"tags"`
	Managed    bool              `json:"managed"`
	ReadOnly   bool              `json:"read_only"`
}

func toCloudInstanceWire(i provision.Instance) cloudInstanceWire {
	tags := i.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	return cloudInstanceWire{ID: i.ID, Name: i.Name, Region: i.Region, State: string(i.State), RawState: i.RawState, Shape: i.Shape,
		OCPUs: i.OCPUs, MemoryGB: i.MemoryGB, PublicIPv4: i.PublicIPv4, CreatedAt: i.CreatedAt, Tags: tags,
		Managed: i.Managed, ReadOnly: i.ReadOnly || !i.Managed}
}

type cloudInstanceDetailWire struct {
	cloudInstanceWire
	PrivateIPv4    string   `json:"private_ipv4,omitempty"`
	Zone           string   `json:"zone,omitempty"`
	ImageID        string   `json:"image_id,omitempty"`
	BootVolumeGB   float64  `json:"boot_volume_gb,omitempty"`
	Project        string   `json:"project,omitempty"`
	Environment    string   `json:"environment,omitempty"`
	ResizeAdvisory string   `json:"resize_advisory,omitempty"`
	Verbs          []string `json:"verbs"`
}

// instanceErrorReply maps a provider error class to one fixed client text; provider output never leaves.
func instanceErrorReply(err error) (int, string) {
	switch {
	case errors.Is(err, provision.ErrInstanceNotFound):
		return http.StatusNotFound, "the instance was not found at the provider"
	case errors.Is(err, provision.ErrInstanceConflict):
		return http.StatusConflict, "the instance is in a state that does not allow this"
	case errors.Is(err, provision.ErrInstanceForbidden):
		return http.StatusForbidden, "the account credential may not do this to the instance"
	case errors.Is(err, provision.ErrInstanceThrottled):
		return http.StatusTooManyRequests, "the provider rate limit was reached, retry later"
	case errors.Is(err, provision.ErrInstanceInvalid):
		return http.StatusUnprocessableEntity, "the provider rejected the request"
	case errors.Is(err, provision.ErrInstanceUnsupported):
		return http.StatusBadRequest, "this provider cannot do that"
	case errors.Is(err, provision.ErrInstanceReadOnly):
		return http.StatusForbidden, "the instance is not managed here and is read only"
	case errors.Is(err, provision.ErrInstanceCapacity):
		return http.StatusConflict, "the provider has no capacity for this"
	}
	return http.StatusBadGateway, "the provider call failed"
}

func (rt *Router) writeInstanceError(w http.ResponseWriter, err error, account string) {
	status, msg := instanceErrorReply(err)
	if status == http.StatusBadGateway {
		rt.logger.Warn("api: cloud instance provider call failed", slog.String("account", account), slog.String("class", "unclassified"))
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "30")
	}
	writeError(w, status, msg)
}

// allowedRegions reads the optional per account region allowlist; empty means every region.
func allowedRegions(a store.CloudAccount) []string {
	var cfg struct {
		AllowedRegions []string `json:"allowed_regions"`
	}
	_ = json.Unmarshal(a.Config, &cfg)
	return cfg.AllowedRegions
}

func regionAllowed(a store.CloudAccount, region string) bool {
	allow := allowedRegions(a)
	return len(allow) == 0 || region == "" || slices.Contains(allow, region)
}

// accountAWSCredentials builds the provision credentials for an AWS account, wrapping role assumption.
func (rt *Router) accountAWSCredentials(ctx context.Context, a store.CloudAccount) (provision.AWSCredentials, error) {
	token, err := rt.accountCredential(ctx, a)
	if err != nil {
		return provision.AWSCredentials{}, err
	}
	if isAWSVaultAccount(a) {
		var cfg cloudconn.Config
		_ = json.Unmarshal(a.Config, &cfg)
		k, _, kerr := rt.awsProvider().VaultKeys(ctx, awsTarget(a, token))
		if kerr != nil {
			return provision.AWSCredentials{}, kerr
		}
		return provision.AWSCredentials{Region: cfg.Region, AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken}, nil
	}
	if isAWSSSOAccount(a) {
		var cfg cloudconn.Config
		_ = json.Unmarshal(a.Config, &cfg)
		k, _, kerr := rt.awsProvider().SSOKeys(ctx, awsTarget(a, token))
		if kerr != nil {
			return provision.AWSCredentials{}, kerr
		}
		return provision.AWSCredentials{Region: cfg.Region, AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken}, nil
	}
	if isAWSRoleAccount(a) {
		var cfg cloudconn.Config
		_ = json.Unmarshal(a.Config, &cfg)
		raw, _ := json.Marshal(provision.AWSCredentials{Region: cfg.Region, RoleARN: cfg.RoleARN, ExternalID: token, UseAmbientCredentials: true}) //nolint:gosec // sealed shape, never logged
		token = string(raw)
	}
	return provision.ParseAWSCredentials(token)
}

// cloudInstanceManager builds the driver for an account, or answers the client itself.
func (rt *Router) cloudInstanceManager(w http.ResponseWriter, r *http.Request, a store.CloudAccount) (provision.InstanceManager, bool) {
	if a.Status == store.CloudAccountStatusDisabled {
		writeError(w, http.StatusConflict, "cloud account is disabled")
		return nil, false
	}
	if a.Provider == "aws" {
		creds, err := rt.accountAWSCredentials(r.Context(), a)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no usable credential is stored for this account")
			return nil, false
		}
		return provision.NewAWSInstances(creds, rt.brand.ShortName, provision.AWSInstanceSettings{AllowedRegions: allowedRegions(a)}), true
	}
	p, ok, status, msg := rt.resolveNodeProvisionerForAccount(r.Context(), a.Provider, a.ID)
	if !ok {
		writeError(w, status, msg)
		return nil, false
	}
	im, ok := p.(provision.InstanceManager)
	if !ok {
		writeError(w, http.StatusBadRequest, "this provider does not support instance management yet")
		return nil, false
	}
	return im, true
}

func cloudInstanceFromPath(r *http.Request) cloudResource {
	return cloudInstanceResource(r.PathValue("id"), "", r.PathValue("iid"))
}

// loadInstance resolves the account, driver and current detail, enforcing the region allowlist.
func (rt *Router) loadInstance(w http.ResponseWriter, r *http.Request) (store.CloudAccount, provision.InstanceManager, provision.InstanceDetail, bool) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return a, nil, provision.InstanceDetail{}, false
	}
	im, ok := rt.cloudInstanceManager(w, r, a)
	if !ok {
		return a, nil, provision.InstanceDetail{}, false
	}
	d, err := im.DescribeInstance(r.Context(), r.PathValue("iid"))
	if err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return a, nil, d, false
	}
	if !regionAllowed(a, d.Region) {
		writeError(w, http.StatusNotFound, "the instance was not found at the provider")
		return a, nil, d, false
	}
	// A compartment outside the account answers 404, the same as an unknown id.
	if d.CompartmentID != "" && !oracleCompartmentAllowed(a, d.CompartmentID) {
		writeError(w, http.StatusNotFound, "the instance was not found at the provider")
		return a, nil, d, false
	}
	return a, im, d, true
}

// loadWritableInstance also refuses unmanaged instances, which stay read only.
func (rt *Router) loadWritableInstance(w http.ResponseWriter, r *http.Request, verb provision.InstanceVerb) (store.CloudAccount, provision.InstanceManager, provision.InstanceDetail, bool) {
	a, im, d, ok := rt.loadInstance(w, r)
	if !ok {
		return a, nil, d, false
	}
	if d.ReadOnly || !d.Managed {
		writeError(w, http.StatusForbidden, "the instance is not managed here and is read only")
		return a, nil, d, false
	}
	if verb != "" && !provision.InstanceCapabilities(im).Supports(verb) {
		writeError(w, http.StatusBadRequest, "this provider cannot do that")
		return a, nil, d, false
	}
	return a, im, d, true
}

func (rt *Router) handleListCloudInstances(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	region, compartment := r.URL.Query().Get("region"), r.URL.Query().Get("compartment")
	if !regionAllowed(a, region) {
		writeError(w, http.StatusForbidden, "this region is not allowed for the account")
		return
	}
	if !oracleCompartmentAllowed(a, compartment) {
		writeError(w, http.StatusForbidden, "this compartment does not belong to the account")
		return
	}
	regions := []string{region}
	if region == "" && len(allowedRegions(a)) > 0 {
		regions = allowedRegions(a)
	}
	ttl := time.Duration(rt.tunableInt(r.Context(), runtimesettings.KeyCloudInstancesCacheTTLSecs, "APP_CLOUD_INSTANCES_CACHE_TTL_SECONDS", 30)) * time.Second
	state := rt.cloudInstanceState()
	key := a.ID + "|" + strings.Join(regions, ",") + "|" + compartment
	state.mu.Lock()
	hit, cached := state.lists[key]
	state.mu.Unlock()
	if cached && time.Since(hit.at) < ttl && r.URL.Query().Get("fresh") == "" {
		writeJSON(w, http.StatusOK, hit.rows)
		return
	}
	im, ok := rt.cloudInstanceManager(w, r, a)
	if !ok {
		return
	}
	rows := []cloudInstanceWire{}
	for _, rg := range regions {
		list, err := im.ListInstances(r.Context(), provision.InstanceScope{Region: rg, Compartment: compartment})
		if err != nil {
			rt.writeInstanceError(w, err, a.ID)
			return
		}
		for _, i := range list {
			if regionAllowed(a, i.Region) {
				rows = append(rows, toCloudInstanceWire(i))
			}
		}
	}
	state.mu.Lock()
	state.lists[key] = cloudInstanceList{at: time.Now(), rows: rows}
	state.mu.Unlock()
	writeJSON(w, http.StatusOK, rows)
}

func (rt *Router) handleGetCloudInstance(w http.ResponseWriter, r *http.Request) {
	_, im, d, ok := rt.loadInstance(w, r)
	if !ok {
		return
	}
	verbs := []string{}
	for _, v := range provision.InstanceCapabilities(im).Verbs() {
		verbs = append(verbs, string(v))
	}
	writeJSON(w, http.StatusOK, cloudInstanceDetailWire{cloudInstanceWire: toCloudInstanceWire(d.Instance), PrivateIPv4: d.PrivateIPv4, Zone: d.Zone,
		ImageID: d.ImageID, BootVolumeGB: d.BootVolumeGB, Project: d.Project, Environment: d.Environment, ResizeAdvisory: d.ResizeAdvisory, Verbs: verbs})
}

func (rt *Router) auditInstance(r *http.Request, action string, a store.CloudAccount, id string, extra map[string]any) {
	detail := map[string]any{"account": a.ID, "instance": id}
	for k, v := range extra {
		detail[k] = v
	}
	rt.auditCloud(r, action, detail)
	rt.cloudInstanceState().drop(a.ID)
}

type cloudInstanceActionBody struct {
	Action string `json:"action"`
}

func (rt *Router) handleCloudInstanceAction(w http.ResponseWriter, r *http.Request) {
	var body cloudInstanceActionBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	kind := provision.InstanceActionKind(body.Action)
	a, im, d, ok := rt.loadWritableInstance(w, r, "")
	if !ok {
		return
	}
	if !provision.InstanceCapabilities(im).SupportsAction(kind) {
		writeError(w, http.StatusBadRequest, "this provider cannot do that")
		return
	}
	if err := im.InstanceAction(r.Context(), d.ID, kind); err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return
	}
	rt.auditInstance(r, "cloud.instance."+body.Action, a, d.ID, map[string]any{"region": d.Region})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "action": body.Action})
}

type cloudInstanceResizeBody struct {
	OCPUs        float64 `json:"ocpus,omitempty"`
	MemoryGB     float64 `json:"memory_gb,omitempty"`
	InstanceType string  `json:"instance_type,omitempty"`
}

// checkResizePrice applies the provision monthly price cap to a new instance type; an unpriced type fails closed.
func (rt *Router) checkResizePrice(ctx context.Context, a store.CloudAccount, region, instanceType string) string {
	capText := rt.provisionSetting(ctx, runtimesettings.KeyProvisionMaxMonthlyPrice, "APP_PROVISION_MAX_MONTHLY_PRICE_USD")
	if capText == "" || instanceType == "" {
		return ""
	}
	maxUSD, err := strconv.ParseFloat(capText, 64)
	if err != nil || math.IsNaN(maxUSD) || maxUSD < 0 {
		return "the provision price cap is not readable"
	}
	est, err := rt.estimateInstancePrice(ctx, a, region, instanceType)
	if err != nil || est.Currency != "USD" {
		if rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeyProvisionAllowUnknownPrice) {
			return ""
		}
		return "the provider lists no usable price for this size, so the price cap cannot be checked"
	}
	if est.Monthly > maxUSD {
		return fmt.Sprintf("size price %.2f USD per month is above the cap of %.2f, raise provision.max_monthly_price_usd", est.Monthly, maxUSD)
	}
	return ""
}

// awsResizeQuota is implemented by the AWS driver; known is false when the quota is unreadable.
type awsResizeQuota interface {
	ResizeQuota(ctx context.Context, id, newType string) (limit, projected float64, known bool, err error)
}

// checkResizeQuota refuses a resize that would pass the provider allowance, with fixed texts.
// Oracle counts A1 use against the operator limits, AWS reads the vCPU service quota.
func (rt *Router) checkResizeQuota(ctx context.Context, a store.CloudAccount, im provision.InstanceManager, d provision.InstanceDetail, body cloudInstanceResizeBody) string {
	switch a.Provider {
	case "oracle":
		if !provision.IsOracleA1Shape(d.Shape) {
			return ""
		}
		src, ok := im.(oracleUsageSource)
		if !ok {
			return "the Oracle allowance cannot be checked for this account, so the resize is refused"
		}
		usage, err := src.OracleUsage(ctx, d.Region)
		if err != nil {
			return "the Oracle allowance could not be checked, so the resize is refused"
		}
		maxOCPU := rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1OCPU, "APP_PROVISION_ORACLE_MAX_A1_OCPU")
		maxMem := rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, "APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB")
		if usage.A1OCPU-d.OCPUs+body.OCPUs > float64(maxOCPU) {
			return fmt.Sprintf("Oracle A1 OCPU limit: the resize would pass provision.oracle.max_a1_ocpu (%d)", maxOCPU)
		}
		if usage.A1MemoryGB-d.MemoryGB+body.MemoryGB > float64(maxMem) {
			return fmt.Sprintf("Oracle A1 memory limit: the resize would pass provision.oracle.max_a1_memory_gb (%d)", maxMem)
		}
	case "aws":
		q, ok := im.(awsResizeQuota)
		if !ok || body.InstanceType == "" {
			return ""
		}
		limit, projected, known, err := q.ResizeQuota(ctx, d.ID, body.InstanceType)
		switch {
		case err != nil:
			return "the vCPU quota could not be checked, so the resize is refused"
		case known && projected > limit:
			return fmt.Sprintf("the resize would use %.0f vCPU in the region against a quota of %.0f", projected, limit)
		}
	}
	return ""
}

func (rt *Router) handleCloudInstanceResize(w http.ResponseWriter, r *http.Request) {
	var body cloudInstanceResizeBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.InstanceType == "" && body.OCPUs <= 0 && body.MemoryGB <= 0 {
		writeError(w, http.StatusBadRequest, "give an instance type or ocpus and memory")
		return
	}
	a, im, d, ok := rt.loadWritableInstance(w, r, provision.VerbResize)
	if !ok {
		return
	}
	if msg := rt.checkResizePrice(r.Context(), a, d.Region, body.InstanceType); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if msg := rt.checkResizeQuota(r.Context(), a, im, d, body); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := im.ResizeInstance(r.Context(), d.ID, provision.ResizeSpec{OCPUs: body.OCPUs, MemoryGB: body.MemoryGB, InstanceType: body.InstanceType}); err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return
	}
	rt.auditInstance(r, "cloud.instance.resize", a, d.ID, map[string]any{"region": d.Region, "from": d.Shape, "to": body.InstanceType, "ocpus": body.OCPUs, "memory_gb": body.MemoryGB})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

type cloudInstanceTagsBody struct {
	Tags map[string]string `json:"tags"`
}

func (rt *Router) handleCloudInstanceTags(w http.ResponseWriter, r *http.Request) {
	var body cloudInstanceTagsBody
	if err := decodeJSONBody(r, &body); err != nil || len(body.Tags) == 0 {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for k := range body.Tags {
		if k == "" || len(k) > 128 || len(body.Tags[k]) > 256 {
			writeError(w, http.StatusBadRequest, "a tag key or value is too long or empty")
			return
		}
	}
	a, im, d, ok := rt.loadWritableInstance(w, r, provision.VerbTags)
	if !ok {
		return
	}
	if err := im.SetInstanceTags(r.Context(), d.ID, body.Tags); err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return
	}
	keys := make([]string, 0, len(body.Tags))
	for k := range body.Tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	rt.auditInstance(r, "cloud.instance.tags", a, d.ID, map[string]any{"region": d.Region, "keys": keys})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// instanceWorkloads counts what the control plane still places on the node backed by this instance.
func (rt *Router) instanceWorkloads(ctx context.Context, accountID, instanceID string) (int, error) {
	if rt.nodeProvisions == nil {
		return 0, nil
	}
	rows, err := rt.nodeProvisions.ListNodeProvisions(ctx)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, p := range rows {
		if p.ProviderServerID != instanceID || p.NodeID == "" || (p.AccountID != "" && p.AccountID != accountID) {
			continue
		}
		svcs, err := rt.apps.ListDesiredServicesByNode(ctx, p.NodeID)
		if err != nil {
			return 0, err
		}
		dbs, err := rt.databases.ListDesiredDatabasesByNode(ctx, p.NodeID)
		if err != nil {
			return 0, err
		}
		total += len(svcs) + len(dbs)
	}
	return total, nil
}

func (rt *Router) handleCloudInstanceTerminate(w http.ResponseWriter, r *http.Request) {
	keep := r.URL.Query().Get("keep_boot_volume") == "true"
	a, im, d, ok := rt.loadWritableInstance(w, r, provision.VerbTerminate)
	if !ok {
		return
	}
	if keep && !provision.InstanceCapabilities(im).Supports(provision.VerbKeepBootVol) {
		writeError(w, http.StatusBadRequest, "this provider cannot keep the boot volume")
		return
	}
	n, err := rt.instanceWorkloads(r.Context(), a.ID, d.ID)
	if err != nil {
		rt.internalError(w, "api: cloud instance workload check failed", err, slog.String("account", a.ID))
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "workloads still run on this node, drain it first")
		return
	}
	if err := im.TerminateInstance(r.Context(), d.ID, keep); err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return
	}
	rt.auditInstance(r, "cloud.instance.terminated", a, d.ID, map[string]any{"region": d.Region, "keep_boot_volume": keep})
	w.WriteHeader(http.StatusNoContent)
}

// estimateInstancePrice prices one instance type through the provider price list; AWS only for now.
func (rt *Router) estimateInstancePrice(ctx context.Context, a store.CloudAccount, region, instanceType string) (cloudspend.PriceEstimate, error) {
	if a.Provider != "aws" {
		return cloudspend.PriceEstimate{}, cloudspend.ErrUnsupported
	}
	rd, err := rt.cloudSpendReader(ctx, a)
	if err != nil {
		return cloudspend.PriceEstimate{}, err
	}
	pe, ok := rd.(interface {
		EstimatePrice(context.Context, cloudspend.PriceQuery) (cloudspend.PriceEstimate, error)
	})
	if !ok {
		return cloudspend.PriceEstimate{}, cloudspend.ErrUnsupported
	}
	return pe.EstimatePrice(ctx, cloudspend.PriceQuery{ServiceCode: "AmazonEC2", Region: region, HoursPerMonth: 730,
		Filters: map[string]string{"instanceType": instanceType, "tenancy": "Shared", "operatingSystem": "Linux", "preInstalledSw": "NA", "capacitystatus": "Used"}})
}

type cloudInstanceEventWire struct {
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"`
	Status  string    `json:"status"`
	Message string    `json:"message,omitempty"`
}

type cloudInstanceEventsWire struct {
	State  string                   `json:"state"`
	Since  time.Time                `json:"since"`
	Events []cloudInstanceEventWire `json:"events"`
}

// handleCloudInstanceEvents lists provider side events inside the runtime window.
// A provider without the capability answers 200 with state unsupported and no rows.
func (rt *Router) handleCloudInstanceEvents(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	days := rt.tunableInt(r.Context(), runtimesettings.KeyCloudInstancesEventsWindowDays, "APP_CLOUD_INSTANCES_EVENTS_WINDOW_DAYS", 7)
	out := cloudInstanceEventsWire{State: "unsupported", Since: time.Now().UTC().AddDate(0, 0, -days), Events: []cloudInstanceEventWire{}}
	if a.Provider != "aws" && a.Provider != "oracle" && a.Provider != "digitalocean" && a.Provider != "hetzner" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	_, im, d, ok := rt.loadInstance(w, r)
	if !ok {
		return
	}
	if !provision.InstanceCapabilities(im).Supports(provision.VerbEvents) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	rows, err := im.InstanceEvents(r.Context(), d.ID, out.Since)
	if err != nil {
		rt.writeInstanceError(w, err, a.ID)
		return
	}
	out.State = "ok"
	for _, e := range rows {
		out.Events = append(out.Events, cloudInstanceEventWire{Time: e.Time, Kind: e.Kind, Status: e.Status, Message: e.Message})
	}
	writeJSON(w, http.StatusOK, out)
}
