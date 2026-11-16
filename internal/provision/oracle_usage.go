package provision

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// oracleListLimit is OCI's page size; longer lists are followed through the
// opc-next-page header up to oracleMaxPages, then refused.
const oracleListLimit = 1000

// Oracle usage scopes: which compartments the counts cover.
const (
	OracleScopeTenancy     = "tenancy"
	OracleScopeCompartment = "compartment"
	// OracleScopeAccessible is a partial tenancy view: only the compartments
	// the key may read, so hidden ones are not counted.
	OracleScopeAccessible = "accessible compartments"
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
	ID           string            `json:"id"`
	FreeformTags map[string]string `json:"freeformTags"`
	Shape        string            `json:"shape"`
	State        string            `json:"lifecycleState"`
	ShapeConfig  struct {
		Ocpus       float64 `json:"ocpus"`
		MemoryInGBs float64 `json:"memoryInGBs"`
	} `json:"shapeConfig"`
}

// IsOracleA1Shape reports an Ampere A1 shape name.
func IsOracleA1Shape(shape string) bool { return strings.HasPrefix(shape, "VM.Standard.A1") }

// IsOracleE2MicroShape reports the AMD micro shape name.
func IsOracleE2MicroShape(shape string) bool { return shape == "VM.Standard.E2.1.Micro" }

func oracleMaxPages() int {
	if n, err := strconv.Atoi(os.Getenv("APP_PROVISION_ORACLE_MAX_PAGES")); err == nil && n > 0 {
		return n
	}
	return 20
}

// listAll follows opc-next-page so a short page with a next-page header is
// never taken as the whole list.
func listAll[T any](ctx context.Context, c *httpClient, path string) ([]T, error) {
	var all []T
	page := ""
	for i := 0; i < oracleMaxPages(); i++ {
		p := path
		if page != "" {
			p += "&page=" + url.QueryEscape(page)
		}
		var rows []T
		hdr, err := c.doHeaders(ctx, "GET", p, nil, nil, &rows)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		page = hdr.Get("opc-next-page")
		if page == "" {
			return all, nil
		}
	}
	return nil, errors.New("provision: oracle list is too long to count")
}

// OracleUsage counts A1 and micro instances across the tenancy when the key
// may read every compartment. The compartment-only fallback needs the
// AllowCompartmentScope setting and only follows a 403 or 404.
func (o *Oracle) OracleUsage(ctx context.Context, region string) (OracleUsage, error) {
	region = o.regionOr(region)
	comps, scope, err := o.readableCompartments(ctx, region)
	if err != nil {
		var pe *ProviderError
		if !o.settings.AllowCompartmentScope || !errors.As(err, &pe) || (pe.Status != 403 && pe.Status != 404) {
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

func (o *Oracle) listCompartments(ctx context.Context, region, level string) ([]string, error) {
	q := url.Values{}
	q.Set("compartmentId", o.cred.TenancyOCID)
	q.Set("compartmentIdInSubtree", "true")
	q.Set("accessLevel", level)
	q.Set("lifecycleState", "ACTIVE")
	q.Set("limit", strconv.Itoa(oracleListLimit))
	comps, err := listAll[struct {
		ID string `json:"id"`
	}](ctx, o.identity(region), "/compartments?"+q.Encode())
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(comps))
	for _, c := range comps {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

// readableCompartments returns the tenancy root plus every accessible active
// compartment. Scope is tenancy only when ACCESSIBLE matches ANY; otherwise
// hidden compartments exist and the result is labelled accessible.
func (o *Oracle) readableCompartments(ctx context.Context, region string) ([]string, string, error) {
	accessible, err := o.listCompartments(ctx, region, "ACCESSIBLE")
	if err != nil {
		return nil, "", err
	}
	all, err := o.listCompartments(ctx, region, "ANY")
	if err != nil {
		return nil, "", err
	}
	have := map[string]bool{}
	for _, id := range accessible {
		have[id] = true
	}
	scope := OracleScopeTenancy
	for _, id := range all {
		if !have[id] {
			scope = OracleScopeAccessible
			break
		}
	}
	if scope == OracleScopeAccessible && !o.settings.AllowCompartmentScope {
		return nil, "", errors.New("provision: oracle key cannot read every compartment, turn on provision.oracle.allow_compartment_scope to count only the readable ones")
	}
	return append([]string{o.cred.TenancyOCID}, accessible...), scope, nil
}

func (o *Oracle) listInstances(ctx context.Context, region, compartment string) ([]ociInstanceRow, error) {
	path := "/instances?limit=" + strconv.Itoa(oracleListLimit) + "&compartmentId=" + url.QueryEscape(compartment)
	return listAll[ociInstanceRow](ctx, o.iaas(region), path)
}
