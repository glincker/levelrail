package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// License classes shown on the marketplace. The site prints the class label, never a judgment.
const (
	ClassPermissive      = "permissive"
	ClassCopyleft        = "copyleft"
	ClassNetworkCopyleft = "network-copyleft"
	ClassSourceAvailable = "source-available"
	ClassProprietary     = "proprietary"
)

// Image source classes: who owns the registry namespace an image comes from.
const (
	SourceUpstream  = "upstream"
	SourceOfficial  = "official"
	SourceRebuilder = "rebuilder"
	SourceCommunity = "community"
)

// Entry lifecycle states.
const (
	StatusActive     = "active"
	StatusDeprecated = "deprecated"
	StatusYanked     = "yanked"
)

// knownLicenses maps SPDX ids to their class, which keeps the class field honest.
var knownLicenses = map[string]string{
	"MIT": ClassPermissive, "Apache-2.0": ClassPermissive, "BSD-2-Clause": ClassPermissive, "BSD-3-Clause": ClassPermissive,
	"ISC": ClassPermissive, "0BSD": ClassPermissive, "Unlicense": ClassPermissive, "Zlib": ClassPermissive, "CC0-1.0": ClassPermissive,
	"GPL-2.0-only": ClassCopyleft, "GPL-2.0-or-later": ClassCopyleft, "GPL-3.0-only": ClassCopyleft, "GPL-3.0-or-later": ClassCopyleft,
	"LGPL-2.1-only": ClassCopyleft, "LGPL-2.1-or-later": ClassCopyleft, "LGPL-3.0-only": ClassCopyleft, "LGPL-3.0-or-later": ClassCopyleft,
	"MPL-2.0": ClassCopyleft, "EPL-2.0": ClassCopyleft,
	"AGPL-3.0-only": ClassNetworkCopyleft, "AGPL-3.0-or-later": ClassNetworkCopyleft,
	// Bare ids are the deprecated SPDX forms, kept for upstreams whose license text states no suffix.
	"GPL-2.0": ClassCopyleft, "GPL-3.0": ClassCopyleft, "AGPL-3.0": ClassNetworkCopyleft,
	"EUPL-1.2": ClassCopyleft, "PostgreSQL": ClassPermissive,
	"SSPL-1.0": ClassSourceAvailable, "BUSL-1.1": ClassSourceAvailable, "Elastic-2.0": ClassSourceAvailable,
}

var (
	licenseRefPattern = regexp.MustCompile(`^LicenseRef-[A-Za-z0-9.-]+$`)
	licenseExprSplit  = regexp.MustCompile(`\s+(?:AND|OR|WITH)\s+`)
	httpsURLPattern   = regexp.MustCompile(`^https://[^\s/]+(/\S*)?$`)
	githubRepoPattern = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9][A-Za-z0-9-]*)/[A-Za-z0-9._-]+$`)
	bareMajorPattern  = regexp.MustCompile(`^v?\d+$`)
	shortTagPattern   = regexp.MustCompile(`^v?\d+\.\d+$`)
	fetchPattern      = regexp.MustCompile(`\b(curl|wget|git clone|pip3? install|npm (i|install)|apk add|apt(-get)? (install|update))\b`)
)

var mutableTags = map[string]bool{
	"latest": true, "stable": true, "edge": true, "main": true, "master": true,
	"nightly": true, "dev": true, "beta": true, "rc": true,
}

// Maintainer exception lists. Each entry needs a reason; the lists start empty
// and a review that grants one adds a line here.
var (
	shortTagExceptions = map[string]string{
		"languagetool": "the community image erikvl87/languagetool publishes X.Y tags only (6.8, last pushed 2026-06-15), no X.Y.Z tags exist",
	}
	communityImageExceptions = map[string]string{
		"languagetool":      "upstream publishes no server image; erikvl87/languagetool is the maintained community image, linux/amd64 and arm64",
		"rss-bridge":        "the project publishes its image under its own Docker Hub org rssbridge, while its source org is RSS-Bridge",
		"hyperdx":           "the project's own Docker Hub org is hyperdx while its source org is hyperdxio",
		"opensearch":        "the project publishes its image under its own Docker Hub org opensearchproject, while its source org is opensearch-project",
		"redash":            "the project publishes its image under its own Docker Hub org redash, while its source org is getredash",
		"node-exporter":     "the project publishes its image under the Docker Hub org prom, while its source org is prometheus",
		"pushgateway":       "the project publishes its image under the Docker Hub org prom, while its source org is prometheus",
		"blackbox-exporter": "the project publishes its image under the Docker Hub org prom, while its source org is prometheus",
	}
	fetchExceptions = map[string]string{}
)

// rebuilderNamespaces are registries that rebuild upstream software under their own name.
var rebuilderNamespaces = []string{"lscr.io/linuxserver/", "ghcr.io/linuxserver/", "linuxserver/"}

func (m Manifest) validateMarketplaceFields() []error {
	var errs []error
	if m.SourceRepo != "" && !githubRepoPattern.MatchString(m.SourceRepo) && !httpsURLPattern.MatchString(m.SourceRepo) {
		errs = append(errs, fmt.Errorf("source_repo %q must be an https URL", m.SourceRepo))
	}
	if m.License != "" {
		errs = append(errs, validateLicense(m.License, m.LicenseClass)...)
	} else if m.LicenseClass != "" {
		errs = append(errs, fmt.Errorf("license_class is set but license is empty"))
	}
	if m.LicenseURL != "" && !httpsURLPattern.MatchString(m.LicenseURL) {
		errs = append(errs, fmt.Errorf("license_url %q must be an https URL", m.LicenseURL))
	}
	switch m.HostedUse {
	case "", "allowed", "restricted", "unknown":
	default:
		errs = append(errs, fmt.Errorf("hosted_use %q must be allowed, restricted or unknown", m.HostedUse))
	}
	errs = append(errs, m.validateListingBlock()...)
	switch m.Status {
	case "", StatusActive:
	case StatusDeprecated, StatusYanked:
		if strings.TrimSpace(m.StatusReason) == "" {
			errs = append(errs, fmt.Errorf("status %s needs a status_reason", m.Status))
		}
	default:
		errs = append(errs, fmt.Errorf("status %q must be active, deprecated or yanked", m.Status))
	}
	for _, a := range m.Aliases {
		if !idPattern.MatchString(a) || a == m.ID {
			errs = append(errs, fmt.Errorf("alias %q must be a lowercase id different from the template id", a))
		}
	}
	if h := m.Healthcheck; h != nil {
		switch h.Kind {
		case "", "http", "tcp":
		default:
			errs = append(errs, fmt.Errorf("healthcheck kind %q must be http or tcp", h.Kind))
		}
		if h.Expect != 0 && h.Expect != 401 && h.Expect != 403 {
			errs = append(errs, fmt.Errorf("healthcheck expect %d must be 401 or 403 (200 to 399 is the default)", h.Expect))
		}
	}
	return errs
}

func validateLicense(license, class string) []error {
	var errs []error
	parts := licenseExprSplit.Split(strings.TrimSpace(license), -1)
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, "()"))
		if _, ok := knownLicenses[p]; !ok && !licenseRefPattern.MatchString(p) {
			errs = append(errs, fmt.Errorf("license %q is not a known SPDX id; use one of the known ids or LicenseRef-<name>", p))
		}
	}
	if class == "" {
		return errs
	}
	switch class {
	case ClassPermissive, ClassCopyleft, ClassNetworkCopyleft, ClassSourceAvailable, ClassProprietary:
	default:
		return append(errs, fmt.Errorf("license_class %q must be permissive, copyleft, network-copyleft, source-available or proprietary", class))
	}
	if len(parts) == 1 {
		want, known := knownLicenses[parts[0]]
		switch {
		case known && want != class:
			errs = append(errs, fmt.Errorf("license %s is %s, not %s", parts[0], want, class))
		case !known && class != ClassSourceAvailable && class != ClassProprietary:
			errs = append(errs, fmt.Errorf("license %s must use class source-available or proprietary", parts[0]))
		}
	}
	return errs
}

func (m Manifest) validateListingBlock() []error {
	l := m.Listing
	if l == nil {
		return nil
	}
	var errs []error
	switch l.By {
	case ListingCommunity, ListingMaintainers:
		if l.VendorOrg != "" {
			errs = append(errs, fmt.Errorf("listing vendor_org is only for by: vendor"))
		}
	case ListingVendor:
		if l.VendorOrg == "" {
			errs = append(errs, fmt.Errorf("listing by vendor needs vendor_org"))
		}
	default:
		errs = append(errs, fmt.Errorf("listing by %q must be community, vendor or maintainers", l.By))
	}
	switch l.Proof {
	case "", "org-member", "dns":
	default:
		errs = append(errs, fmt.Errorf("listing proof %q must be org-member or dns", l.Proof))
	}
	return errs
}

// ImageSourceClass says who owns the namespace of an image ref relative to the source repo.
func ImageSourceClass(ref, sourceRepo string) string {
	img, _ := SplitImageRef(ref)
	img = strings.TrimPrefix(img, "docker.io/")
	for _, ns := range rebuilderNamespaces {
		if strings.HasPrefix(img, ns) {
			return SourceRebuilder
		}
	}
	if !strings.Contains(img, "/") || strings.HasPrefix(img, "library/") {
		return SourceOfficial
	}
	org := ""
	if m := githubRepoPattern.FindStringSubmatch(sourceRepo); m != nil {
		org = strings.ToLower(m[1])
	}
	host := strings.Split(img, "/")
	if strings.Contains(host[0], ".") { // registry host form: ghcr.io/org/app
		host = host[1:]
	}
	if org != "" && len(host) > 0 && strings.ToLower(host[0]) == org {
		return SourceUpstream
	}
	return SourceCommunity
}

// ValidateSubmission applies the stricter marketplace rules to a manifest.
// Entries that predate the marketplace are exempt (legacy_manifests.txt).
func (m Manifest) ValidateSubmission() error {
	var errs []error
	req := map[string]string{"source_repo": m.SourceRepo, "license": m.License, "license_class": m.LicenseClass, "status": m.Status}
	for k, v := range req {
		if v == "" {
			errs = append(errs, fmt.Errorf("missing %s", k))
		}
	}
	if m.Listing == nil {
		errs = append(errs, fmt.Errorf("missing listing"))
	}
	if m.Healthcheck == nil {
		errs = append(errs, fmt.Errorf("missing healthcheck"))
	}
	if m.SourceRepo != "" && !githubRepoPattern.MatchString(m.SourceRepo) {
		errs = append(errs, fmt.Errorf("source_repo must look like https://github.com/<org>/<repo>"))
	}
	f, err := compose.Parse([]byte(m.Compose))
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for name, svc := range f.Services {
		errs = append(errs, m.checkImage(name, svc.Image)...)
		for _, v := range svc.Volumes {
			if v.Name == "" {
				errs = append(errs, fmt.Errorf("service %s: bind mounts are not allowed in submitted templates, use a named volume", name))
			}
		}
	}
	errs = append(errs, m.checkFetch()...)
	return errors.Join(errs...)
}

func (m Manifest) checkImage(service, ref string) []error {
	var errs []error
	_, tag := SplitImageRef(ref)
	switch {
	case mutableTags[strings.ToLower(tag)] || tag == "":
		errs = append(errs, fmt.Errorf("service %s: image %s needs an exact version tag, not %q", service, ref, tag))
	case !strings.ContainsAny(tag, "0123456789"):
		errs = append(errs, fmt.Errorf("service %s: tag %q contains no version number", service, tag))
	case bareMajorPattern.MatchString(tag):
		errs = append(errs, fmt.Errorf("service %s: tag %q is a bare major version, pin the full version", service, tag))
	case shortTagPattern.MatchString(tag) && shortTagExceptions[m.ID] == "":
		errs = append(errs, fmt.Errorf("service %s: tag %q is not a full version; use X.Y.Z or ask a maintainer for an exception", service, tag))
	}
	if ImageSourceClass(ref, m.SourceRepo) == SourceCommunity && communityImageExceptions[m.ID] == "" {
		errs = append(errs, fmt.Errorf("service %s: image %s is not from the upstream namespace, a Docker Official Image or an allowed rebuilder", service, ref))
	}
	return errs
}

func (m Manifest) checkFetch() []error {
	if fetchExceptions[m.ID] != "" {
		return nil
	}
	var doc struct {
		Services map[string]struct {
			Entrypoint any `yaml:"entrypoint"`
			Command    any `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(m.Compose), &doc); err != nil {
		return nil
	}
	var errs []error
	for name, svc := range doc.Services {
		for _, v := range []any{svc.Entrypoint, svc.Command} {
			if loc := fetchPattern.FindString(fmt.Sprint(v)); loc != "" {
				errs = append(errs, fmt.Errorf("service %s: entrypoint or command runs %q; templates may not download or install anything at start", name, loc))
			}
		}
	}
	return errs
}

// ValidateSubmissions loads every manifest under dir and applies
// ValidateSubmission to those whose id is not in the legacy exemption list.
func ValidateSubmissions(fsys fs.FS, dir string, legacy map[string]bool, only map[string]bool) error {
	ms, err := LoadManifests(fsys, dir)
	errs := []error{err}
	for _, m := range ms {
		if legacy[m.ID] || (len(only) > 0 && !only[m.ID]) {
			continue
		}
		if e := m.ValidateSubmission(); e != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path.Join(dir, m.ID), e))
		}
	}
	return errors.Join(errs...)
}
