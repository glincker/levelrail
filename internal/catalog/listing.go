package catalog

// Who stands behind a catalog entry.
const (
	ListingMaintainers = "maintainers"
	ListingCommunity   = "community"
	ListingVendor      = "vendor"
)

// Listing is who stands behind a catalog entry.
type Listing struct {
	By        string
	VendorOrg string
	Proof     string
}

// ListingFor returns the entry's listing. A manifest may set it; contrib
// YAML files are community entries; everything else is maintainer curated.
func ListingFor(t Template) Listing {
	if t.Manifest != nil && t.Manifest.Listing != nil {
		l := t.Manifest.Listing
		return Listing{By: l.By, VendorOrg: l.VendorOrg, Proof: l.Proof}
	}
	for _, c := range contribTemplates {
		if c.ID == t.ID {
			return Listing{By: ListingCommunity}
		}
	}
	return Listing{By: ListingMaintainers}
}
