package provision

import (
	"context"
	"time"
)

// Section states shared by discovery and spend answers. A scope_missing
// section always names the scope to add, never a bare 403.
const (
	SectionOK           = "ok"
	SectionScopeMissing = "scope_missing"
	SectionUnavailable  = "unavailable"
)

// EnrolledNodeRef points at the Levelrail node a discovered machine maps to.
type EnrolledNodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProviderResource is one machine in a provider account, provider-neutral.
// ManagedByLevelrail and EnrolledNode are filled by the API layer, never by
// the provider adapter.
type ProviderResource struct {
	ID                    string           `json:"id"`
	Name                  string           `json:"name"`
	Region                string           `json:"region"`
	Size                  string           `json:"size"`
	Status                string           `json:"status"`
	PublicIPv4            string           `json:"public_ipv4"`
	CreatedAt             time.Time        `json:"created_at"`
	Tags                  []string         `json:"tags"`
	EstimatedMonthlyPrice *float64         `json:"estimated_monthly_price"`
	Currency              string           `json:"currency"`
	ManagedByLevelrail    bool             `json:"managed_by_levelrail"`
	EnrolledNode          *EnrolledNodeRef `json:"enrolled_node"`
}

// ResourceLister is optional on a provisioner: only providers that can list
// the machines in an account implement it. Read-only, never mutates.
type ResourceLister interface {
	ListResources(ctx context.Context) ([]ProviderResource, error)
}

// BillingReader is optional on a provisioner: it reads the billed amounts
// the provider exposes. Each section carries its own state so a missing
// scope on one does not hide the other.
type BillingReader interface {
	ReadBilling(ctx context.Context) (SpendBalance, SpendInvoice)
}

// SpendValue is one number with where it came from and how old it is.
type SpendValue struct {
	Value    string     `json:"value"`
	Source   string     `json:"source"`
	DataAsOf *time.Time `json:"data_as_of"`
}

// SpendSection is the state shared by every spend section.
type SpendSection struct {
	State string `json:"state"`
	Needs string `json:"needs,omitempty"`
}

// SpendBalance is the account balance block.
type SpendBalance struct {
	SpendSection
	AccountBalance     *SpendValue `json:"account_balance"`
	MonthToDateBalance *SpendValue `json:"month_to_date_balance"`
	MonthToDateUsage   *SpendValue `json:"month_to_date_usage"`
	GeneratedAt        *time.Time  `json:"generated_at"`
}

// SpendInvoice summarises the most recent invoice.
type SpendInvoice struct {
	SpendSection
	Amount      *SpendValue `json:"amount"`
	InvoiceID   string      `json:"invoice_id,omitempty"`
	InvoiceUUID string      `json:"invoice_uuid,omitempty"`
	Description string      `json:"description,omitempty"`
	Date        *time.Time  `json:"date"`
}

// SpendEstimate is the monthly total of list prices for discovered machines.
type SpendEstimate struct {
	SpendSection
	MonthlyTotal  *SpendValue `json:"monthly_total"`
	ResourceCount int         `json:"resource_count"`
	PricedCount   int         `json:"priced_count"`
}

// SpendReport is the provider-neutral spend answer.
type SpendReport struct {
	Provider    string        `json:"provider"`
	AccountID   string        `json:"account_id"`
	Currency    string        `json:"currency"`
	GeneratedAt time.Time     `json:"generated_at"`
	Cached      bool          `json:"cached"`
	Balance     SpendBalance  `json:"balance"`
	LastInvoice SpendInvoice  `json:"last_invoice"`
	Estimate    SpendEstimate `json:"estimate"`
}
