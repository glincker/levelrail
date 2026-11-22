package provision

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// InstanceActionKind is a power action every driver maps to its own verb.
type InstanceActionKind string

// The power actions an InstanceManager may support. A driver lists the ones
// it can do in its CapabilityMatrix, the rest are hidden, never errors.
const (
	InstanceActionStart      InstanceActionKind = "start"
	InstanceActionStop       InstanceActionKind = "stop"
	InstanceActionSoftStop   InstanceActionKind = "soft_stop"
	InstanceActionReboot     InstanceActionKind = "reboot"
	InstanceActionSoftReboot InstanceActionKind = "soft_reboot"
)

// InstanceState is the provider state folded into a small shared vocabulary.
type InstanceState string

// Shared instance states. The raw provider value stays on Instance.RawState.
const (
	InstanceStatePending     InstanceState = "pending"
	InstanceStateRunning     InstanceState = "running"
	InstanceStateStopping    InstanceState = "stopping"
	InstanceStateStopped     InstanceState = "stopped"
	InstanceStateTerminating InstanceState = "terminating"
	InstanceStateTerminated  InstanceState = "terminated"
	InstanceStateUnknown     InstanceState = "unknown"
)

// InstanceScope narrows a listing. Empty fields mean the account default.
type InstanceScope struct {
	Region string
	// Compartment is only read by drivers that have compartments.
	Compartment string
}

// Instance is the neutral list row.
type Instance struct {
	ID         string
	Name       string
	Region     string
	State      InstanceState
	RawState   string
	Shape      string
	OCPUs      float64
	MemoryGB   float64
	PublicIPv4 string
	// PrivateIPv4 is set on list rows by drivers that resolve it; detail has its own copy.
	PrivateIPv4 string
	// IPNote says why the addresses are empty when the caller may not read them.
	IPNote    string
	CreatedAt time.Time
	Tags      map[string]string
	// Managed is true when the instance carries this product's ownership tags.
	Managed bool
	// ReadOnly marks an instance the driver lists but refuses to change.
	ReadOnly bool
}

// InstanceDetail adds what only the detail page needs.
type InstanceDetail struct {
	Instance
	PrivateIPv4    string
	Zone           string
	ImageID        string
	BootVolumeGB   float64
	Project        string
	Environment    string
	ResizeAdvisory string
	// CompartmentID is set by drivers with compartments so callers can enforce scope.
	CompartmentID string
}

// InstanceEvent is one provider-side event or operation on an instance.
type InstanceEvent struct {
	Time    time.Time
	Kind    string
	Status  string
	Message string
}

// ResizeSpec carries what a driver needs to resize. Flexible-shape drivers read
// OCPUs and MemoryGB, drivers with fixed types read InstanceType.
type ResizeSpec struct {
	OCPUs        float64
	MemoryGB     float64
	InstanceType string
}

// InstanceManager is optional: a driver that implements it gets the instance
// detail actions. It is deliberately separate from Provisioner so the other
// drivers stay untouched.
type InstanceManager interface {
	ListInstances(ctx context.Context, scope InstanceScope) ([]Instance, error)
	DescribeInstance(ctx context.Context, id string) (InstanceDetail, error)
	InstanceAction(ctx context.Context, id string, kind InstanceActionKind) error
	ResizeInstance(ctx context.Context, id string, spec ResizeSpec) error
	// SetInstanceTags upserts the given tags; an empty value removes the key.
	SetInstanceTags(ctx context.Context, id string, tags map[string]string) error
	TerminateInstance(ctx context.Context, id string, keepBootVolume bool) error
	InstanceEvents(ctx context.Context, id string, since time.Time) ([]InstanceEvent, error)
}

// InstanceVerb names one thing a driver may or may not do.
type InstanceVerb string

// Every verb the UI gates on. Adding a capability is one entry here plus the
// driver listing it.
const (
	VerbList          InstanceVerb = "list"
	VerbDescribe      InstanceVerb = "describe"
	VerbStart         InstanceVerb = "start"
	VerbStop          InstanceVerb = "stop"
	VerbSoftStop      InstanceVerb = "soft_stop"
	VerbReboot        InstanceVerb = "reboot"
	VerbSoftReboot    InstanceVerb = "soft_reboot"
	VerbResize        InstanceVerb = "resize"
	VerbTags          InstanceVerb = "tags"
	VerbTerminate     InstanceVerb = "terminate"
	VerbKeepBootVol   InstanceVerb = "keep_boot_volume"
	VerbEvents        InstanceVerb = "events"
	VerbResizeByShape InstanceVerb = "resize_by_type"
)

// actionVerbs is the one table mapping an action kind to its verb.
var actionVerbs = map[InstanceActionKind]InstanceVerb{
	InstanceActionStart:      VerbStart,
	InstanceActionStop:       VerbStop,
	InstanceActionSoftStop:   VerbSoftStop,
	InstanceActionReboot:     VerbReboot,
	InstanceActionSoftReboot: VerbSoftReboot,
}

// CapabilityMatrix reports which verbs a driver supports.
type CapabilityMatrix map[InstanceVerb]bool

// NewCapabilityMatrix builds a matrix with exactly the listed verbs on.
func NewCapabilityMatrix(verbs ...InstanceVerb) CapabilityMatrix {
	m := CapabilityMatrix{}
	for _, v := range verbs {
		m[v] = true
	}
	return m
}

// Supports reports whether the verb is on; unknown verbs are off.
func (c CapabilityMatrix) Supports(v InstanceVerb) bool { return c[v] }

// SupportsAction reports whether the power action is on.
func (c CapabilityMatrix) SupportsAction(k InstanceActionKind) bool {
	v, ok := actionVerbs[k]
	return ok && c[v]
}

// Verbs lists the supported verbs in a stable order.
func (c CapabilityMatrix) Verbs() []InstanceVerb {
	out := make([]InstanceVerb, 0, len(c))
	for v, on := range c {
		if on {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// InstanceCapabilityReporter is implemented by managers that can say what they
// support.
type InstanceCapabilityReporter interface {
	InstanceCapabilities() CapabilityMatrix
}

// InstanceCapabilities returns the manager's matrix. A manager that does not
// report gets the full set, so an old driver never loses actions by accident.
func InstanceCapabilities(m InstanceManager) CapabilityMatrix {
	if r, ok := m.(InstanceCapabilityReporter); ok {
		return r.InstanceCapabilities()
	}
	return NewCapabilityMatrix(VerbList, VerbDescribe, VerbStart, VerbStop, VerbSoftStop,
		VerbReboot, VerbSoftReboot, VerbResize, VerbTags, VerbTerminate, VerbKeepBootVol, VerbEvents)
}

// Instance error classes. Callers branch with errors.Is, never on provider text.
var (
	ErrInstanceNotFound    = errors.New("provision: instance not found")
	ErrInstanceConflict    = errors.New("provision: instance is in a state that does not allow this")
	ErrInstanceForbidden   = errors.New("provision: credential may not do this to the instance")
	ErrInstanceThrottled   = errors.New("provision: provider rate limit reached")
	ErrInstanceInvalid     = errors.New("provision: provider rejected the request")
	ErrInstanceUnsupported = errors.New("provision: this provider cannot do that")
	ErrInstanceReadOnly    = errors.New("provision: instance is not managed here and is read only")
	ErrInstanceCapacity    = errors.New("provision: provider has no capacity for this")
)

// classifiedError keeps the cause for errors.As while errors.Is matches the
// class. Error() carries only the class and code, never provider text.
type classifiedError struct {
	class error
	code  string
	cause error
}

func (e *classifiedError) Error() string {
	if e.code == "" {
		return e.class.Error()
	}
	return fmt.Sprintf("%s (%s)", e.class.Error(), e.code)
}

func (e *classifiedError) Is(target error) bool { return target == e.class }
func (e *classifiedError) Unwrap() error        { return e.cause }

func classify(class error, code string, cause error) error {
	return &classifiedError{class: class, code: code, cause: cause}
}
