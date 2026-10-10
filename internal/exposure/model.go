// Package exposure answers "which published container ports can the
// internet reach" and manages comment-tagged DOCKER-USER rules that
// restrict them. Published ports skip the INPUT chain, so ufw alone says
// nothing about them.
package exposure

// Class is how reachable one published port is.
type Class string

// Reachability classes.
const (
	ClassLoopback   Class = "loopback"
	ClassPrivate    Class = "private"
	ClassRestricted Class = "restricted"
	ClassExposed    Class = "exposed"
	ClassUnknown    Class = "unknown"
)

// Severity ranks a finding for the operator.
type Severity string

// Finding severities.
const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
	SeverityInfo   Severity = "info"
)

// OwnerKind says who owns the container publishing a port.
type OwnerKind string

// Owner kinds.
const (
	OwnerDatabase  OwnerKind = "database"
	OwnerApp       OwnerKind = "app"
	OwnerUnmanaged OwnerKind = "unmanaged"
)

// ImageKind is a coarse guess at what a container runs, from its image name
// and published port.
type ImageKind string

// Image kinds.
const (
	KindDatastore ImageKind = "datastore"
	KindSearch    ImageKind = "search"
	KindAdmin     ImageKind = "admin"
	KindDockerAPI ImageKind = "docker_api"
	KindWeb       ImageKind = "web"
	KindOther     ImageKind = "other"
)

// Owner is the platform resource behind a container, if any.
type Owner struct {
	Kind OwnerKind `json:"kind"`
	Name string    `json:"name,omitempty"`
	// Intentional is true when the operator explicitly asked for public
	// access (a database's PubliclyAccessible, an app's public bind).
	Intentional bool `json:"intentional,omitempty"`
}

// Container is one running container's published ports, as the audit reads them.
type Container struct {
	ID      string
	Name    string
	Image   string
	Owner   Owner
	Ports   []PortBinding
	Running bool
}

// PortBinding is one published port as Docker reports it.
type PortBinding struct {
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      string
}

// Finding is the audit result for one published port of one container.
type Finding struct {
	Container     string    `json:"container"`
	Image         string    `json:"image"`
	Owner         Owner     `json:"owner"`
	ImageKind     ImageKind `json:"image_kind"`
	Protocol      string    `json:"protocol"`
	HostPort      int       `json:"host_port"`
	ContainerPort int       `json:"container_port"`
	Binds         []string  `json:"binds"`
	Class         Class     `json:"class"`
	Severity      Severity  `json:"severity"`
	// AllowedSources lists the sources a matching rule lets through, when Class is restricted.
	AllowedSources []string `json:"allowed_sources,omitempty"`
	// Rules are the matching DOCKER-USER rules, verbatim.
	Rules []string `json:"rules,omitempty"`
	// Managed is true when the matching rule is one this platform created.
	Managed        bool   `json:"managed"`
	Explanation    string `json:"explanation"`
	Recommendation string `json:"recommendation,omitempty"`
	// CanRestrict is false when this audit cannot offer the guided fix, with the reason.
	CanRestrict       bool   `json:"can_restrict"`
	CannotRestrictWhy string `json:"cannot_restrict_reason,omitempty"`
}

// NeedsAttention reports whether the finding is worth surfacing: published
// beyond the host's private interfaces and not provably restricted.
func (f Finding) NeedsAttention() bool {
	return f.Class == ClassExposed || f.Class == ClassUnknown
}
