package provision

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// oracleListLimit is OCI's page maximum. A full page cannot be proven
// complete without paging, so it is treated as unreadable and refused.
const oracleListLimit = 1000

// Oracle usage scopes: which compartments the counts cover.
const (
	OracleScopeTenancy     = "tenancy"
	OracleScopeCompartment = "compartment"
)

// OracleUsage is what existing non-terminated instances already hold, whether
// or not Levelrail created them.
type OracleUsage struct {
	A1OCPU     float64
	A1MemoryGB float64
	E2Micro    int
	// Scope says how wide the count was, so a refusal can state it.
	Scope string
}

type ociInstanceRow struct {
	Shape       string `json:"shape"`
	State       string `json:"lifecycleState"`
	ShapeConfig struct {
		Ocpus       float64 `json:"ocpus"`
		MemoryInGBs float64 `json:"memoryInGBs"`
	} `json:"shapeConfig"`
}

// IsOracleA1Shape reports an Ampere A1 shape name.
func IsOracleA1Shape(shape string) bool { return strings.HasPrefix(shape, "VM.Standard.A1") }

// IsOracleE2MicroShape reports the AMD micro shape name.
func IsOracleE2MicroShape(shape string) bool { return shape == "VM.Standard.E2.1.Micro" }

// OracleUsage counts A1 and micro instances across the tenancy when the key
// may read every compartment, else in the configured compartment only.
func (o *Oracle) OracleUsage(ctx context.Context, region string) (OracleUsage, error) {
	region = o.regionOr(region)
	comps, err := o.readableCompartments(ctx, region)
	scope := OracleScopeTenancy
	if err != nil {
		var pe *ProviderError
		if !errors.As(err, &pe) || pe.Status < 400 || pe.Status >= 500 {
			return OracleUsage{}, err
		}
		comps, scope = []string{o.cred.CompartmentOCID}, OracleScopeCompartment
	}
	out := OracleUsage{Scope: scope}
	for _, c := range comps {
		rows, err := o.listInstances(ctx, region, c)
		if err != nil {
			return OracleUsage{}, err
		}
		for _, r := range rows {
			if r.State == "TERMINATED" {
				continue
			}
			switch {
			case IsOracleA1Shape(r.Shape):
				out.A1OCPU += r.ShapeConfig.Ocpus
				out.A1MemoryGB += r.ShapeConfig.MemoryInGBs
			case IsOracleE2MicroShape(r.Shape):
				out.E2Micro++
			}
		}
	}
	return out, nil
}

// readableCompartments lists the tenancy root plus every accessible active
// compartment. A 4xx means the key cannot read the tenancy tree.
func (o *Oracle) readableCompartments(ctx context.Context, region string) ([]string, error) {
	q := url.Values{}
	q.Set("compartmentId", o.cred.TenancyOCID)
	q.Set("compartmentIdInSubtree", "true")
	q.Set("accessLevel", "ACCESSIBLE")
	q.Set("lifecycleState", "ACTIVE")
	q.Set("limit", "1000")
	var comps []struct {
		ID string `json:"id"`
	}
	if err := o.identity(region).do(ctx, "GET", "/compartments?"+q.Encode(), nil, &comps); err != nil {
		return nil, err
	}
	if len(comps) >= oracleListLimit {
		return nil, errors.New("provision: oracle compartment list is too long to count")
	}
	out := []string{o.cred.TenancyOCID}
	for _, c := range comps {
		out = append(out, c.ID)
	}
	return out, nil
}

func (o *Oracle) listInstances(ctx context.Context, region, compartment string) ([]ociInstanceRow, error) {
	var rows []ociInstanceRow
	path := "/instances?limit=1000&compartmentId=" + url.QueryEscape(compartment)
	if err := o.iaas(region).do(ctx, "GET", path, nil, &rows); err != nil {
		return nil, err
	}
	if len(rows) >= oracleListLimit {
		return nil, errors.New("provision: oracle instance list is too long to count")
	}
	return rows, nil
}
