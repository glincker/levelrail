package api

import (
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// templateListing says who stands behind a catalog entry.
type templateListing struct {
	By        string `json:"by"`
	VendorOrg string `json:"vendor_org,omitempty"`
}

// templateFacts are the marketplace facts shared by the list item and the
// detail. Every field is optional so older clients keep working, and
// hosted_use is deliberately absent.
type templateFacts struct {
	License      *catalog.License      `json:"license,omitempty"`
	Listing      *templateListing      `json:"listing,omitempty"`
	Trust        *catalog.Trust        `json:"trust,omitempty"`
	Verification *catalog.Verification `json:"verification,omitempty"`
	Arch         map[string]string     `json:"arch,omitempty"`
	// Status is active, deprecated or yanked; a deprecated entry stays listed and installable.
	Status       string `json:"status,omitempty"`
	StatusReason string `json:"status_reason,omitempty"`
	ReplacedBy   string `json:"replaced_by,omitempty"`
	// Preview marks a catalog entry with no real-engine pass yet; it stays installable.
	Preview bool `json:"preview,omitempty"`
}

func factsFor(id string) templateFacts {
	m, ok := catalog.MarketplaceByID(id)
	if !ok {
		return templateFacts{}
	}
	return templateFacts{
		License:      m.License,
		Listing:      &templateListing{By: m.Listing.By, VendorOrg: m.Listing.VendorOrg},
		Trust:        &m.Trust,
		Verification: m.Verification,
		Arch:         m.Arch,
		Status:       m.Status,
		StatusReason: m.StatusReason,
		ReplacedBy:   m.ReplacedBy,
		Preview:      catalog.IsPreview(id),
	}
}

// publicManifest drops hosted_use, which is catalog bookkeeping and not shown to users.
func publicManifest(m *catalog.Manifest) *catalog.Manifest {
	if m == nil || m.HostedUse == "" {
		return m
	}
	c := *m
	c.HostedUse = ""
	return &c
}

type templateFilter struct {
	licenseClass      string
	verifiedOnly      bool
	arch              string
	includeDeprecated bool
}

var licenseClasses = map[string]bool{
	catalog.ClassPermissive: true, catalog.ClassCopyleft: true, catalog.ClassNetworkCopyleft: true,
	catalog.ClassSourceAvailable: true, catalog.ClassProprietary: true,
}

// parseTemplateFilter reads the optional list filters; nothing is hidden unless asked.
func parseTemplateFilter(r *http.Request) (templateFilter, string) {
	q := r.URL.Query()
	f := templateFilter{licenseClass: q.Get("license_class"), arch: q.Get("arch"), includeDeprecated: true}
	if f.licenseClass != "" && !licenseClasses[f.licenseClass] {
		return f, "license_class must be permissive, copyleft, network-copyleft, source-available or proprietary"
	}
	if f.arch != "" && f.arch != "amd64" && f.arch != "arm64" {
		return f, "arch must be amd64 or arm64"
	}
	for _, p := range []struct {
		name string
		dst  *bool
	}{{"verified", &f.verifiedOnly}, {"include_deprecated", &f.includeDeprecated}} {
		switch strings.ToLower(q.Get(p.name)) {
		case "":
		case "true", "1":
			*p.dst = true
		case "false", "0":
			*p.dst = false
		default:
			return f, p.name + " must be true or false"
		}
	}
	return f, ""
}

func (f templateFilter) keep(facts templateFacts) bool {
	if f.licenseClass != "" && (facts.License == nil || facts.License.Class != f.licenseClass) {
		return false
	}
	if f.verifiedOnly && (facts.Trust == nil || facts.Trust.Level == catalog.TrustListed) {
		return false
	}
	if f.arch != "" {
		if s := facts.Arch[f.arch]; s != "tested" && s != "published" {
			return false
		}
	}
	return f.includeDeprecated || facts.Status == "" || facts.Status == catalog.StatusActive
}
