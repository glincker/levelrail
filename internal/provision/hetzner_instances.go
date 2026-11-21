package provision

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

var _ InstanceManager = (*Hetzner)(nil)

const (
	// hetznerManagedLabel marks servers this product made; it is never edited through the tag API.
	hetznerManagedLabel = "levelrail"
	hetznerListPageSize = 50
	hetznerListMaxPages = 100
)

// hetznerActionPaths maps a power action to the server action endpoint.
var hetznerActionPaths = map[InstanceActionKind]string{
	InstanceActionStart:      "poweron",
	InstanceActionStop:       "poweroff",
	InstanceActionSoftStop:   "shutdown",
	InstanceActionReboot:     "reset",
	InstanceActionSoftReboot: "reboot",
}

var hetznerStates = map[string]InstanceState{
	"initializing": InstanceStatePending,
	"starting":     InstanceStatePending,
	"migrating":    InstanceStatePending,
	"rebuilding":   InstanceStatePending,
	"running":      InstanceStateRunning,
	"stopping":     InstanceStateStopping,
	"off":          InstanceStateStopped,
	"deleting":     InstanceStateTerminating,
}

// InstanceCapabilities lists what Hetzner servers support. Boot volume
// retention is off: the root disk is part of the server.
func (h *Hetzner) InstanceCapabilities() CapabilityMatrix {
	return NewCapabilityMatrix(VerbList, VerbDescribe, VerbStart, VerbStop, VerbSoftStop,
		VerbReboot, VerbSoftReboot, VerbResize, VerbResizeByShape, VerbTags, VerbTerminate, VerbEvents)
}

type hetznerServerDoc struct {
	ID         int64             `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Created    time.Time         `json:"created"`
	Labels     map[string]string `json:"labels"`
	ServerType struct {
		Name   string  `json:"name"`
		Cores  float64 `json:"cores"`
		Memory float64 `json:"memory"`
		Disk   float64 `json:"disk"`
	} `json:"server_type"`
	Datacenter struct {
		Name     string `json:"name"`
		Location struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"datacenter"`
	Image *struct {
		Name string `json:"name"`
	} `json:"image"`
	PublicNet struct {
		IPv4 struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
	PrivateNet []struct {
		IP string `json:"ip"`
	} `json:"private_net"`
}

func (s hetznerServerDoc) instance() Instance {
	st, ok := hetznerStates[s.Status]
	if !ok {
		st = InstanceStateUnknown
	}
	_, managed := s.Labels[hetznerManagedLabel]
	return Instance{
		ID: strconv.FormatInt(s.ID, 10), Name: s.Name, Region: s.Datacenter.Location.Name, State: st, RawState: s.Status,
		Shape: s.ServerType.Name, OCPUs: s.ServerType.Cores, MemoryGB: s.ServerType.Memory,
		PublicIPv4: s.PublicNet.IPv4.IP, CreatedAt: s.Created, Tags: s.Labels, Managed: managed,
	}
}

// ListInstances lists servers across every page, optionally of one location.
func (h *Hetzner) ListInstances(ctx context.Context, scope InstanceScope) ([]Instance, error) {
	var out []Instance
	for page := 1; page <= hetznerListMaxPages; page++ {
		var res struct {
			Servers []hetznerServerDoc `json:"servers"`
			hetznerPagination
		}
		path := "/servers?per_page=" + strconv.Itoa(hetznerListPageSize) + "&page=" + strconv.Itoa(page)
		if err := h.client.do(ctx, http.MethodGet, path, nil, &res); err != nil {
			return nil, classifyHTTPInstance(err, false)
		}
		for _, s := range res.Servers {
			if scope.Region == "" || s.Datacenter.Location.Name == scope.Region {
				out = append(out, s.instance())
			}
		}
		if res.Meta.Pagination.NextPage == 0 {
			break
		}
	}
	return out, nil
}

func (h *Hetzner) getServerDoc(ctx context.Context, id string) (hetznerServerDoc, error) {
	if err := validNumericInstanceID(id); err != nil {
		return hetznerServerDoc{}, err
	}
	var res hetznerServerDocResponse
	if err := h.client.do(ctx, http.MethodGet, "/servers/"+id, nil, &res); err != nil {
		return hetznerServerDoc{}, classifyHTTPInstance(err, false)
	}
	return res.Server, nil
}

type hetznerServerDocResponse struct {
	Server hetznerServerDoc `json:"server"`
}

// DescribeInstance reads one server.
func (h *Hetzner) DescribeInstance(ctx context.Context, id string) (InstanceDetail, error) {
	s, err := h.getServerDoc(ctx, id)
	if err != nil {
		return InstanceDetail{}, err
	}
	d := InstanceDetail{
		Instance: s.instance(), Zone: s.Datacenter.Name, BootVolumeGB: s.ServerType.Disk,
		ResizeAdvisory: "The server must be powered off to change its type. The disk is kept the same size so the change can be undone.",
	}
	if len(s.PrivateNet) > 0 {
		d.PrivateIPv4 = s.PrivateNet[0].IP
	}
	if s.Image != nil {
		d.ImageID = s.Image.Name
	}
	return d, nil
}

func (h *Hetzner) writableServer(ctx context.Context, id string) (hetznerServerDoc, error) {
	s, err := h.getServerDoc(ctx, id)
	if err != nil {
		return s, err
	}
	if !s.instance().Managed {
		return s, classify(ErrInstanceReadOnly, "", errors.New("provision: server has no ownership label"))
	}
	return s, nil
}

// InstanceAction runs one power action on a managed server.
func (h *Hetzner) InstanceAction(ctx context.Context, id string, kind InstanceActionKind) error {
	p, ok := hetznerActionPaths[kind]
	if !ok || !h.InstanceCapabilities().SupportsAction(kind) {
		return classify(ErrInstanceUnsupported, string(kind), errors.New("provision: hetzner has no such action"))
	}
	if _, err := h.writableServer(ctx, id); err != nil {
		return err
	}
	return classifyHTTPInstance(h.client.do(ctx, http.MethodPost, "/servers/"+id+"/actions/"+p, nil, nil), true)
}

// ResizeInstance changes the server type without upgrading the disk.
func (h *Hetzner) ResizeInstance(ctx context.Context, id string, spec ResizeSpec) error {
	if spec.InstanceType == "" {
		return classify(ErrInstanceInvalid, "type", errors.New("provision: hetzner resizes by server type"))
	}
	if _, err := h.writableServer(ctx, id); err != nil {
		return err
	}
	body := map[string]any{"server_type": spec.InstanceType, "upgrade_disk": false}
	return classifyHTTPInstance(h.client.do(ctx, http.MethodPost, "/servers/"+id+"/actions/change_type", body, nil), true)
}

// SetInstanceTags merges labels into the server; an empty value removes the key.
func (h *Hetzner) SetInstanceTags(ctx context.Context, id string, tags map[string]string) error {
	s, err := h.writableServer(ctx, id)
	if err != nil {
		return err
	}
	if _, bad := tags[hetznerManagedLabel]; bad {
		return classify(ErrInstanceInvalid, "tag", errors.New("provision: the ownership label cannot be changed"))
	}
	labels := make(map[string]string, len(s.Labels)+len(tags))
	for k, v := range s.Labels {
		labels[k] = v
	}
	for k, v := range tags {
		if v == "" {
			delete(labels, k)
			continue
		}
		labels[k] = v
	}
	return classifyHTTPInstance(h.client.do(ctx, http.MethodPut, "/servers/"+id, map[string]any{"labels": labels}, nil), false)
}

// TerminateInstance deletes the server. Keeping the disk is not possible.
func (h *Hetzner) TerminateInstance(ctx context.Context, id string, keepBootVolume bool) error {
	if keepBootVolume {
		return classify(ErrInstanceUnsupported, string(VerbKeepBootVol), errors.New("provision: a server disk cannot be kept"))
	}
	if _, err := h.writableServer(ctx, id); err != nil {
		return err
	}
	return classifyHTTPInstance(h.client.do(ctx, http.MethodDelete, "/servers/"+id, nil, nil), true)
}

// InstanceEvents reads the server action history newer than since.
func (h *Hetzner) InstanceEvents(ctx context.Context, id string, since time.Time) ([]InstanceEvent, error) {
	if err := validNumericInstanceID(id); err != nil {
		return nil, err
	}
	var res struct {
		Actions []struct {
			Command string    `json:"command"`
			Status  string    `json:"status"`
			Started time.Time `json:"started"`
		} `json:"actions"`
	}
	path := "/servers/" + id + "/actions?sort=started:desc&per_page=" + strconv.Itoa(hetznerListPageSize)
	if err := h.client.do(ctx, http.MethodGet, path, nil, &res); err != nil {
		return nil, classifyHTTPInstance(err, false)
	}
	out := []InstanceEvent{}
	for _, a := range res.Actions {
		if a.Started.Before(since) {
			continue
		}
		out = append(out, InstanceEvent{Time: a.Started, Kind: a.Command, Status: a.Status})
	}
	return out, nil
}
