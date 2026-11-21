package provision

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var _ InstanceManager = (*Oracle)(nil)

// oracleActions maps a power action to the OCI InstanceAction query value.
var oracleActions = map[InstanceActionKind]string{
	InstanceActionStart:      "START",
	InstanceActionStop:       "STOP",
	InstanceActionSoftStop:   "SOFTSTOP",
	InstanceActionReboot:     "RESET",
	InstanceActionSoftReboot: "SOFTRESET",
}

// oracleStates maps lifecycleState to the shared vocabulary.
var oracleStates = map[string]InstanceState{
	"PROVISIONING":   InstanceStatePending,
	"STARTING":       InstanceStatePending,
	"CREATING_IMAGE": InstanceStatePending,
	"MOVING":         InstanceStatePending,
	"RUNNING":        InstanceStateRunning,
	"STOPPING":       InstanceStateStopping,
	"STOPPED":        InstanceStateStopped,
	"TERMINATING":    InstanceStateTerminating,
	"TERMINATED":     InstanceStateTerminated,
}

// InstanceCapabilities reports every verb: OCI has all five power actions.
func (o *Oracle) InstanceCapabilities() CapabilityMatrix {
	return NewCapabilityMatrix(VerbList, VerbDescribe, VerbStart, VerbStop, VerbSoftStop,
		VerbReboot, VerbSoftReboot, VerbResize, VerbTags, VerbTerminate, VerbKeepBootVol, VerbEvents)
}

type ociInstanceDoc struct {
	ID                 string            `json:"id"`
	DisplayName        string            `json:"displayName"`
	State              string            `json:"lifecycleState"`
	Shape              string            `json:"shape"`
	Region             string            `json:"region"`
	AvailabilityDomain string            `json:"availabilityDomain"`
	CompartmentID      string            `json:"compartmentId"`
	TimeCreated        time.Time         `json:"timeCreated"`
	FreeformTags       map[string]string `json:"freeformTags"`
	ShapeConfig        struct {
		Ocpus       float64 `json:"ocpus"`
		MemoryInGBs float64 `json:"memoryInGBs"`
	} `json:"shapeConfig"`
	SourceDetails struct {
		ImageID             string  `json:"imageId"`
		BootVolumeSizeInGBs float64 `json:"bootVolumeSizeInGBs"`
	} `json:"sourceDetails"`
}

func (o *Oracle) instanceFromDoc(d ociInstanceDoc, region string) Instance {
	st, ok := oracleStates[d.State]
	if !ok {
		st = InstanceStateUnknown
	}
	_, managed := d.FreeformTags[o.settings.LabelKey]
	return Instance{
		ID: d.ID, Name: d.DisplayName, Region: region, State: st, RawState: d.State,
		Shape: d.Shape, OCPUs: d.ShapeConfig.Ocpus, MemoryGB: d.ShapeConfig.MemoryInGBs,
		CreatedAt: d.TimeCreated, Tags: d.FreeformTags, Managed: managed,
	}
}

// classifyOracle maps an OCI error to an instance error class by status and
// the provider error code, so callers never read provider text.
func classifyOracle(err error) error {
	var pe *ProviderError
	if err == nil || !errors.As(err, &pe) {
		return err
	}
	switch {
	case pe.ociCode == "IncorrectState" || pe.Status == 409:
		return classify(ErrInstanceConflict, pe.ociCode, err)
	case pe.Status == 404:
		return classify(ErrInstanceNotFound, pe.ociCode, err)
	case pe.Status == 401 || pe.Status == 403:
		return classify(ErrInstanceForbidden, pe.ociCode, err)
	case pe.Status == 429 || pe.ociCode == "TooManyRequests":
		return classify(ErrInstanceThrottled, pe.ociCode, err)
	case pe.ociCode == "LimitExceeded" || isCapacityError(err):
		return classify(ErrInstanceCapacity, pe.ociCode, err)
	case pe.Status == 400:
		return classify(ErrInstanceInvalid, pe.ociCode, err)
	}
	return err
}

// ListInstances lists the non-terminated instances of one compartment and region.
func (o *Oracle) ListInstances(ctx context.Context, scope InstanceScope) ([]Instance, error) {
	region := o.regionOr(scope.Region)
	comp := scope.Compartment
	if comp == "" {
		comp = o.cred.CompartmentOCID
	}
	if !oracleOCIDRe.MatchString(comp) {
		return nil, classify(ErrInstanceInvalid, "compartment", errors.New("provision: oracle compartment OCID is not valid"))
	}
	path := "/instances?limit=" + strconv.Itoa(oracleListLimit) + "&compartmentId=" + url.QueryEscape(comp)
	rows, err := listAll[ociInstanceDoc](ctx, o.iaas(region), path)
	if err != nil {
		return nil, classifyOracle(err)
	}
	out := make([]Instance, 0, len(rows))
	for _, r := range rows {
		if r.State == "TERMINATED" {
			continue
		}
		out = append(out, o.instanceFromDoc(r, region))
	}
	return out, nil
}

// locate returns the client and region where the instance answers. A 404 only
// moves on to the next region the id could belong to.
func (o *Oracle) locate(ctx context.Context, id string) (*httpClient, string, ociInstanceDoc, error) {
	if !validOracleInstanceID(id) {
		return nil, "", ociInstanceDoc{}, classify(ErrInstanceInvalid, "id", errors.New("provision: oracle instance id is not valid"))
	}
	regions, err := o.instanceRegions(ctx, id)
	if err != nil {
		return nil, "", ociInstanceDoc{}, err
	}
	lastErr := classify(ErrInstanceNotFound, "", errors.New("provision: oracle instance not found"))
	for _, region := range regions {
		c := o.iaas(region)
		var doc ociInstanceDoc
		if err := c.do(ctx, "GET", "/instances/"+url.PathEscape(id), nil, &doc); err != nil {
			var pe *ProviderError
			if errors.As(err, &pe) && pe.Status == 404 {
				lastErr = classifyOracle(err)
				continue
			}
			return nil, "", ociInstanceDoc{}, classifyOracle(err)
		}
		if doc.CompartmentID != "" && o.cred.CompartmentOCID != "" && doc.CompartmentID != o.cred.CompartmentOCID {
			return nil, "", ociInstanceDoc{}, classify(ErrInstanceNotFound, "compartment", errors.New("provision: oracle instance is outside the account compartment"))
		}
		return c, region, doc, nil
	}
	return nil, "", ociInstanceDoc{}, lastErr
}

// DescribeInstance reads state, shape, addresses, image and tags.
func (o *Oracle) DescribeInstance(ctx context.Context, id string) (InstanceDetail, error) {
	c, region, doc, err := o.locate(ctx, id)
	if err != nil {
		return InstanceDetail{}, err
	}
	inst := o.instanceFromDoc(doc, region)
	inst.PublicIPv4 = o.publicIP(ctx, c, id)
	d := InstanceDetail{
		Instance: inst, Zone: doc.AvailabilityDomain, ImageID: doc.SourceDetails.ImageID,
		BootVolumeGB: doc.SourceDetails.BootVolumeSizeInGBs, CompartmentID: doc.CompartmentID, PrivateIPv4: o.privateIP(ctx, c, id),
	}
	if doc.ShapeConfig.Ocpus > 0 || strings.Contains(doc.Shape, "Flex") {
		d.ResizeAdvisory = "resizing a flexible shape reboots the instance"
	}
	return d, nil
}

func (o *Oracle) privateIP(ctx context.Context, c *httpClient, instanceID string) string {
	var atts []struct {
		VnicID string `json:"vnicId"`
		State  string `json:"lifecycleState"`
	}
	path := "/vnicAttachments?compartmentId=" + url.QueryEscape(o.cred.CompartmentOCID) + "&instanceId=" + url.QueryEscape(instanceID)
	if err := c.do(ctx, "GET", path, nil, &atts); err != nil {
		return ""
	}
	for _, a := range atts {
		if a.VnicID == "" || (a.State != "" && a.State != "ATTACHED") {
			continue
		}
		var v struct {
			PrivateIP string `json:"privateIp"`
		}
		if err := c.do(ctx, "GET", "/vnics/"+url.PathEscape(a.VnicID), nil, &v); err == nil && v.PrivateIP != "" {
			return v.PrivateIP
		}
	}
	return ""
}

// InstanceAction runs one power action through POST /instances/{id}?action=.
func (o *Oracle) InstanceAction(ctx context.Context, id string, kind InstanceActionKind) error {
	action, ok := oracleActions[kind]
	if !ok {
		return classify(ErrInstanceUnsupported, string(kind), errors.New("provision: oracle has no such action"))
	}
	c, _, _, err := o.locate(ctx, id)
	if err != nil {
		return err
	}
	return classifyOracle(c.do(ctx, "POST", "/instances/"+url.PathEscape(id)+"?action="+action, nil, nil))
}

// ResizeInstance changes OCPU and memory of a flexible shape through PUT
// /instances/{id}. Fixed shapes are refused before any change is sent.
func (o *Oracle) ResizeInstance(ctx context.Context, id string, spec ResizeSpec) error {
	if spec.OCPUs <= 0 || spec.MemoryGB <= 0 {
		return classify(ErrInstanceInvalid, "shapeConfig", errors.New("provision: oracle resize needs ocpus and memory"))
	}
	c, _, doc, err := o.locate(ctx, id)
	if err != nil {
		return err
	}
	if doc.ShapeConfig.Ocpus <= 0 || !strings.Contains(doc.Shape, "Flex") {
		return classify(ErrInstanceUnsupported, "fixed_shape", errors.New("provision: oracle shape is not flexible"))
	}
	body := map[string]any{"shapeConfig": map[string]any{"ocpus": spec.OCPUs, "memoryInGBs": spec.MemoryGB}}
	return classifyOracle(c.do(ctx, "PUT", "/instances/"+url.PathEscape(id), body, nil))
}

// SetInstanceTags merges into the freeform tags, since OCI replaces the whole
// map on update. The ownership label is never changed here.
func (o *Oracle) SetInstanceTags(ctx context.Context, id string, tags map[string]string) error {
	c, _, doc, err := o.locate(ctx, id)
	if err != nil {
		return err
	}
	if _, ok := tags[o.settings.LabelKey]; ok {
		return classify(ErrInstanceInvalid, "reserved_tag", errors.New("provision: oracle ownership tag cannot be changed"))
	}
	merged := map[string]string{}
	for k, v := range doc.FreeformTags {
		merged[k] = v
	}
	for k, v := range tags {
		if v == "" {
			delete(merged, k)
			continue
		}
		merged[k] = v
	}
	return classifyOracle(c.do(ctx, "PUT", "/instances/"+url.PathEscape(id), map[string]any{"freeformTags": merged}, nil))
}

// TerminateInstance deletes the instance; preserveBootVolume follows keepBootVolume.
func (o *Oracle) TerminateInstance(ctx context.Context, id string, keepBootVolume bool) error {
	c, _, _, err := o.locate(ctx, id)
	if err != nil {
		return err
	}
	path := "/instances/" + url.PathEscape(id) + "?preserveBootVolume=" + strconv.FormatBool(keepBootVolume)
	return classifyOracle(c.do(ctx, "DELETE", path, nil, nil))
}

// auditClient returns the Audit service client; the shared test base serves it too.
func (o *Oracle) auditClient(region string) *httpClient {
	if o.iaasBase != "" {
		return newHTTPClientWithSigner(o.iaasBase, o.signer)
	}
	return newHTTPClientWithSigner("https://audit."+region+".oraclecloud.com/20190901", o.signer)
}

// oracleAuditWindow is how far back events are read when the caller gives no start time.
func oracleAuditWindow() time.Duration {
	if n, err := strconv.Atoi(os.Getenv("APP_PROVISION_ORACLE_AUDIT_WINDOW_HOURS")); err == nil && n > 0 {
		return time.Duration(n) * time.Hour
	}
	return 7 * 24 * time.Hour
}

// InstanceEvents reads the compartment's Audit events and keeps the ones for this
// instance, since Core has no work request listing by resource. A 404 is an empty list.
func (o *Oracle) InstanceEvents(ctx context.Context, id string, since time.Time) ([]InstanceEvent, error) {
	_, region, _, err := o.locate(ctx, id)
	if err != nil {
		return nil, err
	}
	end := time.Now().UTC()
	if since.IsZero() {
		since = end.Add(-oracleAuditWindow())
	}
	path := "/auditEvents?compartmentId=" + url.QueryEscape(o.cred.CompartmentOCID) +
		"&startTime=" + url.QueryEscape(since.UTC().Format(time.RFC3339)) + "&endTime=" + url.QueryEscape(end.Format(time.RFC3339))
	rows, err := listAll[struct {
		EventName string    `json:"eventName"`
		EventTime time.Time `json:"eventTime"`
		Data      struct {
			ResourceID string `json:"resourceId"`
			Response   struct {
				Status string `json:"status"`
			} `json:"response"`
		} `json:"data"`
	}](ctx, o.auditClient(region), path)
	if err != nil {
		var pe *ProviderError
		if errors.As(err, &pe) && pe.Status == 404 {
			return []InstanceEvent{}, nil
		}
		return nil, classifyOracle(err)
	}
	out := []InstanceEvent{}
	for _, r := range rows {
		if r.Data.ResourceID != id || r.EventTime.Before(since) {
			continue
		}
		out = append(out, InstanceEvent{Time: r.EventTime, Kind: r.EventName, Status: r.Data.Response.Status})
	}
	return out, nil
}
