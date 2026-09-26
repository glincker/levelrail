package supplychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	topPackageLimit = 10
	topLicenseLimit = 8
)

type spdxDoc struct {
	Packages []struct {
		SPDXID           string `json:"SPDXID"`
		Name             string `json:"name"`
		VersionInfo      string `json:"versionInfo"`
		LicenseConcluded string `json:"licenseConcluded"`
		LicenseDeclared  string `json:"licenseDeclared"`
		ExternalRefs     []struct {
			Type    string `json:"referenceType"`
			Locator string `json:"referenceLocator"`
		} `json:"externalRefs"`
	} `json:"packages"`
}

type cdxDoc struct {
	Components []struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Version  string `json:"version"`
		PURL     string `json:"purl"`
		Licenses []struct {
			License *struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"license"`
			Expression string `json:"expression"`
		} `json:"licenses"`
	} `json:"components"`
}

var errUnknownSBOM = errors.New("supplychain: parse sbom: not an SPDX or CycloneDX document")

// ParseSBOM reads an SPDX or CycloneDX JSON document into its packages.
func ParseSBOM(data []byte) (format string, pkgs []Package, err error) {
	var probe struct {
		SPDXVersion string `json:"spdxVersion"`
		BOMFormat   string `json:"bomFormat"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", nil, fmt.Errorf("supplychain: parse sbom: %w", err)
	}
	switch {
	case probe.SPDXVersion != "":
		pkgs, err = parseSPDX(data)
		return FormatSPDX, pkgs, err
	case strings.EqualFold(probe.BOMFormat, "CycloneDX"):
		pkgs, err = parseCycloneDX(data)
		return FormatCycloneDX, pkgs, err
	}
	return "", nil, errUnknownSBOM
}

func parseSPDX(data []byte) ([]Package, error) {
	var doc spdxDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("supplychain: parse spdx: %w", err)
	}
	var out []Package
	for _, p := range doc.Packages {
		if p.Name == "" || strings.Contains(p.SPDXID, "DocumentRoot") {
			continue
		}
		pkg := Package{Name: p.Name, Version: p.VersionInfo, License: firstLicense(p.LicenseConcluded, p.LicenseDeclared)}
		for _, ref := range p.ExternalRefs {
			if ref.Type == "purl" {
				pkg.Type = purlType(ref.Locator)
				break
			}
		}
		out = append(out, pkg)
	}
	return out, nil
}

func parseCycloneDX(data []byte) ([]Package, error) {
	var doc cdxDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("supplychain: parse cyclonedx: %w", err)
	}
	var out []Package
	for _, c := range doc.Components {
		if c.Name == "" {
			continue
		}
		pkg := Package{Name: c.Name, Version: c.Version, Type: purlType(c.PURL)}
		if pkg.Type == "" {
			pkg.Type = c.Type
		}
		for _, l := range c.Licenses {
			switch {
			case l.Expression != "":
				pkg.License = l.Expression
			case l.License != nil && l.License.ID != "":
				pkg.License = l.License.ID
			case l.License != nil && l.License.Name != "":
				pkg.License = l.License.Name
			}
			if pkg.License != "" {
				break
			}
		}
		out = append(out, pkg)
	}
	return out, nil
}

func firstLicense(candidates ...string) string {
	for _, c := range candidates {
		if c != "" && c != "NOASSERTION" && c != "NONE" {
			return c
		}
	}
	return ""
}

func purlType(purl string) string {
	rest, ok := strings.CutPrefix(purl, "pkg:")
	if !ok {
		return ""
	}
	t, _, _ := strings.Cut(rest, "/")
	return t
}

// Summarize builds the compact view of pkgs.
func Summarize(format string, pkgs []Package) SBOMSummary {
	sum := SBOMSummary{Format: format, PackageCount: len(pkgs)}
	types := map[string]int{}
	licenses := map[string]int{}
	for _, p := range pkgs {
		if p.Type != "" {
			types[p.Type]++
		}
		if p.License == "" {
			sum.Unlicensed++
			continue
		}
		licenses[p.License]++
	}
	for t, n := range types {
		sum.Types = append(sum.Types, TypeCount{Type: t, Count: n})
	}
	sort.Slice(sum.Types, func(i, j int) bool {
		if sum.Types[i].Count != sum.Types[j].Count {
			return sum.Types[i].Count > sum.Types[j].Count
		}
		return sum.Types[i].Type < sum.Types[j].Type
	})
	for l, n := range licenses {
		sum.Licenses = append(sum.Licenses, LicenseCount{License: l, Count: n})
	}
	sort.Slice(sum.Licenses, func(i, j int) bool {
		if sum.Licenses[i].Count != sum.Licenses[j].Count {
			return sum.Licenses[i].Count > sum.Licenses[j].Count
		}
		return sum.Licenses[i].License < sum.Licenses[j].License
	})
	if len(sum.Licenses) > topLicenseLimit {
		sum.Licenses = sum.Licenses[:topLicenseLimit]
	}
	sorted := append([]Package(nil), pkgs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Version < sorted[j].Version
	})
	if len(sorted) > topPackageLimit {
		sorted = sorted[:topPackageLimit]
	}
	sum.TopPackages = sorted
	return sum
}
