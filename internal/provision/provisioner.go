// Package provision creates and tears down cloud VMs at a provider (Hetzner,
// DigitalOcean) so a fresh node can join the fleet through the existing
// join-token enrollment flow, without an operator manually clicking through a
// cloud console first.
package provision

import "context"

// Region is one region/location a provider can create a server in.
type Region struct {
	ID   string
	Name string
}

// Size is one server size/plan a provider offers.
type Size struct {
	ID     string
	Name   string
	VCPUs  int
	Memory int // MB
	Disk   int // GB
	// PriceMonthly is the provider's own listed monthly price, in its
	// native currency string. Empty when the provider does not return a
	// price for this size.
	PriceMonthly string
	Currency     string
}

// ServerStatus is a provider-reported server lifecycle state, normalized
// across providers into a small common vocabulary.
type ServerStatus string

// The three server states GetServer normalizes every provider's own
// status string into.
const (
	ServerStatusPending ServerStatus = "pending"
	ServerStatusRunning ServerStatus = "running"
	ServerStatusError   ServerStatus = "error"
)

// CreateOpts describes the server CreateServer should provision.
type CreateOpts struct {
	Region string
	Size   string
	Name   string
	// UserData is the cloud-init script run on first boot.
	UserData string
}

// Provisioner creates and manages servers at a cloud provider. hetzner.go
// and digitalocean.go are the two real implementations; both talk to their
// provider's REST API directly over net/http, no SDK.
type Provisioner interface {
	ListRegions(ctx context.Context) ([]Region, error)
	ListSizes(ctx context.Context, region string) ([]Size, error)
	// CreateServer returns the provider's own server id and, when the
	// provider assigns one immediately, its public IPv4 address (empty
	// until GetServer reports one, for providers that assign it later).
	CreateServer(ctx context.Context, opts CreateOpts) (serverID, ipAddr string, err error)
	GetServer(ctx context.Context, id string) (status ServerStatus, ipAddr string, err error)
	DeleteServer(ctx context.Context, id string) error
}
