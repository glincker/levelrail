package provision

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var _ InstanceManager = (*DigitalOcean)(nil)

// doNumericIDRe keeps a caller supplied id from reaching the URL path unchecked.
var doNumericIDRe = regexp.MustCompile(`^[0-9]{1,20}$`)

const (
	// doManagedTag marks droplets this product made; it is never edited through the tag API.
	doManagedTag     = "levelrail"
	doListPageSize   = 200
	doListMaxPages   = 50
	doEventsPageSize = 50
)

// doActionTypes maps a power action to the droplet action type.
var doActionTypes = map[InstanceActionKind]string{
	InstanceActionStart:      "power_on",
	InstanceActionStop:       "power_off",
	InstanceActionSoftStop:   "shutdown",
	InstanceActionReboot:     "power_cycle",
	InstanceActionSoftReboot: "reboot",
}

var doStates = map[string]InstanceState{
	"new":     InstanceStatePending,
	"active":  InstanceStateRunning,
	"off":     InstanceStateStopped,
	"archive": InstanceStateTerminated,
}

// InstanceCapabilities lists what droplets support. Boot volume retention is
// off: a droplet disk cannot outlive the droplet.
func (d *DigitalOcean) InstanceCapabilities() CapabilityMatrix {
	return NewCapabilityMatrix(VerbList, VerbDescribe, VerbStart, VerbStop, VerbSoftStop,
		VerbReboot, VerbSoftReboot, VerbResize, VerbResizeByShape, VerbTags, VerbTerminate, VerbEvents)
}

type doDropletDoc struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	SizeSlug string   `json:"size_slug"`
	Vcpus    float64  `json:"vcpus"`
	Memory   float64  `json:"memory"`
	Disk     float64  `json:"disk"`
	Created  string   `json:"created_at"`
	Tags     []string `json:"tags"`
	Region   struct {
		Slug string `json:"slug"`
	} `json:"region"`
	Image struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	} `json:"image"`
	Networks struct {
		V4 []struct {
			IPAddress string `json:"ip_address"`
			Type      string `json:"type"`
		} `json:"v4"`
	} `json:"networks"`
}

func (d doDropletDoc) ip(kind string) string {
	for _, n := range d.Networks.V4 {
		if n.Type == kind {
			return n.IPAddress
		}
	}
	return ""
}

// doTagMap folds DO tag names into the neutral map: "k:v" becomes k=v, a bare name becomes name="".
func doTagMap(tags []string) map[string]string {
	out := make(map[string]string, len(tags))
	for _, t := range tags {
		k, v, _ := strings.Cut(t, ":")
		out[k] = v
	}
	return out
}

func (d doDropletDoc) instance() Instance {
	st, ok := doStates[d.Status]
	if !ok {
		st = InstanceStateUnknown
	}
	created, _ := time.Parse(time.RFC3339, d.Created)
	managed := false
	for _, t := range d.Tags {
		if t == doManagedTag {
			managed = true
		}
	}
	return Instance{
		ID: strconv.FormatInt(d.ID, 10), Name: d.Name, Region: d.Region.Slug, State: st, RawState: d.Status,
		Shape: d.SizeSlug, OCPUs: d.Vcpus, MemoryGB: d.Memory / 1024, PublicIPv4: d.ip("public"),
		CreatedAt: created, Tags: doTagMap(d.Tags), Managed: managed,
	}
}

// classifyHTTPInstance maps a provider status to an instance error class.
// conflict422 is set for state-changing calls, where 422 means a wrong state.
func classifyHTTPInstance(err error, conflict422 bool) error {
	var pe *ProviderError
	if err == nil || !errors.As(err, &pe) {
		return err
	}
	switch {
	case pe.Status == http.StatusNotFound:
		return classify(ErrInstanceNotFound, "", err)
	case pe.Status == http.StatusConflict || pe.Status == http.StatusLocked || (conflict422 && pe.Status == http.StatusUnprocessableEntity):
		return classify(ErrInstanceConflict, "", err)
	case pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden:
		return classify(ErrInstanceForbidden, "", err)
	case pe.Status == http.StatusTooManyRequests:
		return classify(ErrInstanceThrottled, "", err)
	case pe.Status == http.StatusBadRequest || pe.Status == http.StatusUnprocessableEntity:
		return classify(ErrInstanceInvalid, "", err)
	}
	return err
}

func validNumericInstanceID(id string) error {
	if !doNumericIDRe.MatchString(id) {
		return classify(ErrInstanceInvalid, "id", errors.New("provision: instance id is not valid"))
	}
	return nil
}

// ListInstances lists droplets across every page, optionally of one region.
func (d *DigitalOcean) ListInstances(ctx context.Context, scope InstanceScope) ([]Instance, error) {
	var out []Instance
	for page := 1; page <= doListMaxPages; page++ {
		var res struct {
			Droplets []doDropletDoc `json:"droplets"`
			doLinks
		}
		path := "/droplets?per_page=" + strconv.Itoa(doListPageSize) + "&page=" + strconv.Itoa(page)
		if err := d.client.do(ctx, http.MethodGet, path, nil, &res); err != nil {
			return nil, classifyHTTPInstance(err, false)
		}
		for _, dr := range res.Droplets {
			if scope.Region == "" || dr.Region.Slug == scope.Region {
				out = append(out, dr.instance())
			}
		}
		if res.Links.Pages.Next == "" {
			break
		}
	}
	return out, nil
}

func (d *DigitalOcean) getDroplet(ctx context.Context, id string) (doDropletDoc, error) {
	if err := validNumericInstanceID(id); err != nil {
		return doDropletDoc{}, err
	}
	var res doDropletResponseDoc
	if err := d.client.do(ctx, http.MethodGet, "/droplets/"+id, nil, &res); err != nil {
		return doDropletDoc{}, classifyHTTPInstance(err, false)
	}
	return res.Droplet, nil
}

type doDropletResponseDoc struct {
	Droplet doDropletDoc `json:"droplet"`
}

// DescribeInstance reads one droplet.
func (d *DigitalOcean) DescribeInstance(ctx context.Context, id string) (InstanceDetail, error) {
	dr, err := d.getDroplet(ctx, id)
	if err != nil {
		return InstanceDetail{}, err
	}
	img := dr.Image.Slug
	if img == "" && dr.Image.ID != 0 {
		img = strconv.FormatInt(dr.Image.ID, 10)
	}
	return InstanceDetail{
		Instance: dr.instance(), PrivateIPv4: dr.ip("private"), Zone: dr.Region.Slug,
		ImageID: img, BootVolumeGB: dr.Disk,
		ResizeAdvisory: "The droplet must be powered off to resize. The disk is kept the same size so the change can be undone.",
	}, nil
}

func (d *DigitalOcean) writableDroplet(ctx context.Context, id string) (doDropletDoc, error) {
	dr, err := d.getDroplet(ctx, id)
	if err != nil {
		return dr, err
	}
	if !dr.instance().Managed {
		return dr, classify(ErrInstanceReadOnly, "", errors.New("provision: droplet has no ownership tag"))
	}
	return dr, nil
}

// InstanceAction runs one power action on a managed droplet.
func (d *DigitalOcean) InstanceAction(ctx context.Context, id string, kind InstanceActionKind) error {
	typ, ok := doActionTypes[kind]
	if !ok || !d.InstanceCapabilities().SupportsAction(kind) {
		return classify(ErrInstanceUnsupported, string(kind), errors.New("provision: digitalocean has no such action"))
	}
	if _, err := d.writableDroplet(ctx, id); err != nil {
		return err
	}
	err := d.client.do(ctx, http.MethodPost, "/droplets/"+id+"/actions", map[string]any{"type": typ}, nil)
	return classifyHTTPInstance(err, true)
}

// ResizeInstance changes the droplet size slug without growing the disk.
func (d *DigitalOcean) ResizeInstance(ctx context.Context, id string, spec ResizeSpec) error {
	if spec.InstanceType == "" {
		return classify(ErrInstanceInvalid, "size", errors.New("provision: digitalocean resizes by size slug"))
	}
	if _, err := d.writableDroplet(ctx, id); err != nil {
		return err
	}
	body := map[string]any{"type": "resize", "size": spec.InstanceType, "disk": false}
	return classifyHTTPInstance(d.client.do(ctx, http.MethodPost, "/droplets/"+id+"/actions", body, nil), true)
}

func doTagName(k, v string) string {
	if v == "" {
		return k
	}
	return k + ":" + v
}

// SetInstanceTags upserts tags as DO tag names and detaches the removed ones.
// A tag that belongs to no other resource is left in the account, never deleted.
func (d *DigitalOcean) SetInstanceTags(ctx context.Context, id string, tags map[string]string) error {
	dr, err := d.writableDroplet(ctx, id)
	if err != nil {
		return err
	}
	for k := range tags {
		if k == doManagedTag {
			return classify(ErrInstanceInvalid, "tag", errors.New("provision: the ownership tag cannot be changed"))
		}
	}
	cur := doTagMap(dr.Tags)
	res := []map[string]string{{"resource_id": id, "resource_type": "droplet"}}
	detach := func(name string) error {
		rm := map[string]any{"resources": res}
		return d.client.do(ctx, http.MethodDelete, "/tags/"+url.PathEscape(name)+"/resources", rm, nil)
	}
	for k, v := range tags {
		old, has := cur[k]
		if v == "" {
			if has {
				if err := detach(doTagName(k, old)); err != nil {
					return classifyHTTPInstance(err, false)
				}
			}
			continue
		}
		if has {
			if old == v {
				continue
			}
			if err := detach(doTagName(k, old)); err != nil {
				return classifyHTTPInstance(err, false)
			}
		}
		name := doTagName(k, v)
		if err := d.client.do(ctx, http.MethodPost, "/tags", map[string]string{"name": name}, nil); err != nil {
			var pe *ProviderError
			if !errors.As(err, &pe) || pe.Status != http.StatusUnprocessableEntity {
				return classifyHTTPInstance(err, false)
			}
		}
		if err := d.client.do(ctx, http.MethodPost, "/tags/"+url.PathEscape(name)+"/resources", map[string]any{"resources": res}, nil); err != nil {
			return classifyHTTPInstance(err, false)
		}
	}
	return nil
}

// TerminateInstance deletes the droplet. Keeping the disk is not possible.
func (d *DigitalOcean) TerminateInstance(ctx context.Context, id string, keepBootVolume bool) error {
	if keepBootVolume {
		return classify(ErrInstanceUnsupported, string(VerbKeepBootVol), errors.New("provision: a droplet disk cannot be kept"))
	}
	if _, err := d.writableDroplet(ctx, id); err != nil {
		return err
	}
	return classifyHTTPInstance(d.client.do(ctx, http.MethodDelete, "/droplets/"+id, nil, nil), true)
}

// InstanceEvents reads the droplet action history newer than since.
func (d *DigitalOcean) InstanceEvents(ctx context.Context, id string, since time.Time) ([]InstanceEvent, error) {
	if err := validNumericInstanceID(id); err != nil {
		return nil, err
	}
	var res struct {
		Actions []struct {
			Type      string `json:"type"`
			Status    string `json:"status"`
			StartedAt string `json:"started_at"`
		} `json:"actions"`
	}
	path := "/droplets/" + id + "/actions?per_page=" + strconv.Itoa(doEventsPageSize)
	if err := d.client.do(ctx, http.MethodGet, path, nil, &res); err != nil {
		return nil, classifyHTTPInstance(err, false)
	}
	out := []InstanceEvent{}
	for _, a := range res.Actions {
		t, err := time.Parse(time.RFC3339, a.StartedAt)
		if err != nil || t.Before(since) {
			continue
		}
		out = append(out, InstanceEvent{Time: t, Kind: a.Type, Status: a.Status})
	}
	return out, nil
}
