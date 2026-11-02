// Command gen-template-gallery exports internal/catalog.Templates as the
// versioned catalog index (docs/catalog-index.md) that the docs gallery,
// the marketing site and a future catalog API all read.
//
// Usage (from the repo root): go run ./scripts/gen-template-gallery
// Pass -check to exit non-zero instead of writing when the file is stale.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// Service is one compose service as shown on a gallery page.
type Service struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Ports   []int    `json:"ports"`
	Volumes []string `json:"volumes"`
}

// EnvVar is one environment variable name and how it gets its value.
// Values are never exported.
type EnvVar struct {
	Service string `json:"service"`
	Key     string `json:"name"`
	Kind    string `json:"kind"`
}

// SchemaVersion is the version of the catalog index contract. Bump it only
// for a breaking change; additive fields keep the version.
const SchemaVersion = 1

// Image is one service image with the facts tied to its pinned tag.
type Image struct {
	Service     string            `json:"service"`
	Image       string            `json:"image"`
	Tag         string            `json:"tag"`
	Digest      string            `json:"digest,omitempty"`
	SourceClass string            `json:"source_class"`
	Platforms   []string          `json:"platforms,omitempty"`
	Signed      *catalog.SignedBy `json:"signed"`
	Provenance  bool              `json:"provenance"`
	SBOM        bool              `json:"sbom"`
	NonRoot     bool              `json:"non_root"`
}

// License is the stated license and its class.
type License struct {
	SPDX  string `json:"spdx"`
	Class string `json:"class"`
	URL   string `json:"url,omitempty"`
}

// Listing says who stands behind an entry.
type Listing struct {
	By        string `json:"by"`
	VendorOrg string `json:"vendor_org,omitempty"`
	Proof     string `json:"proof,omitempty"`
}

// Trust is the composite level and the CI run that backs it.
type Trust struct {
	Level       string `json:"level"`
	EvidenceURL string `json:"evidence_url,omitempty"`
}

// Verification is the smoke record, bound to a tag and digest, with no dates.
type Verification struct {
	Tag        string   `json:"tag"`
	Digest     string   `json:"digest,omitempty"`
	HTTPStatus int      `json:"http_status,omitempty"`
	Arches     []string `json:"arches"`
	RunURL     string   `json:"run_url,omitempty"`
}

// Entry is one template in the catalog index.
type Entry struct {
	ID               string               `json:"id"`
	Aliases          []string             `json:"aliases"`
	Name             string               `json:"name"`
	Slogan           string               `json:"slogan"`
	Description      string               `json:"description,omitempty"`
	FirstRun         []string             `json:"first_run"`
	Category         string               `json:"category"`
	Status           string               `json:"status"`
	StatusReason     string               `json:"status_reason,omitempty"`
	ReplacedBy       string               `json:"replaced_by,omitempty"`
	SourceRepo       string               `json:"source_repo,omitempty"`
	DocumentationURL string               `json:"documentation_url,omitempty"`
	License          *License             `json:"license,omitempty"`
	HostedUse        string               `json:"hosted_use"`
	Listing          Listing              `json:"listing"`
	Trust            Trust                `json:"trust"`
	Arch             map[string]string    `json:"arch"`
	Verification     *Verification        `json:"verification,omitempty"`
	Admission        *catalog.Admission   `json:"admission,omitempty"`
	Images           []Image              `json:"images"`
	MemoryMiB        int64                `json:"memory_mib,omitempty"`
	RequiresGPU      bool                 `json:"requires_gpu,omitempty"`
	NeedsConfig      bool                 `json:"needs_config"`
	Services         []Service            `json:"services"`
	Env              []EnvVar             `json:"env"`
	Requirements     catalog.Requirements `json:"requirements"`
}

// Index is the whole file: the stable contract the site and a future API share.
type Index struct {
	SchemaVersion int                      `json:"schema_version"`
	Templates     []Entry                  `json:"templates"`
	Tombstones    []catalog.TombstoneEntry `json:"tombstones"`
}

// Trust levels. Every entry is at least listed.
const (
	trustListed         = "listed"
	trustVerified       = "verified"
	trustVerifiedSigned = "verified_signed"
)

// statusPageURL records smoke passes that predate CI run links.
const statusPageURL = "https://github.com/glincker/levelrail/blob/main/docs/templates-status.md"

const (
	kindGenerated = "generated"
	kindRequired  = "required"
	kindPreset    = "preset"
)

var dashReplacer = strings.NewReplacer(" \u2014 ", ", ", "\u2014", ", ", " \u2013 ", ", ", "\u2013", "-")

// sanitize removes em and en dashes from catalog prose.
func sanitize(s string) string {
	return dashReplacer.Replace(s)
}

func detectGen(_, _ string, _ int) (string, error) { return "x", nil }
func detectPersist(_, _, _ string) error           { return nil }

// buildEntry converts one catalog template, classifying env vars by name only.
func buildEntry(t catalog.Template) (Entry, error) {
	e := Entry{
		ID:               t.ID,
		Aliases:          []string{},
		FirstRun:         []string{},
		Name:             sanitize(t.Name),
		Slogan:           sanitize(t.Slogan),
		Description:      sanitize(t.Description),
		Category:         t.Category,
		Status:           catalog.StatusActive,
		DocumentationURL: t.DocumentationURL,
		HostedUse:        "unknown",
		Trust:            Trust{Level: trustListed},
		Images:           []Image{},
		MemoryMiB:        t.RecommendedMemoryBytes / (1 << 20),
		RequiresGPU:      t.RequiresGPU,
		Services:         []Service{},
		Env:              []EnvVar{},
		Requirements:     catalog.ComputeRequirements(t.Compose),
	}
	if m := t.Manifest; m != nil {
		applyManifest(&e, m)
	} else if meta, ok := catalog.MetaFor(t.ID); ok {
		applyMeta(&e, meta)
	}
	l := catalog.ListingFor(t)
	e.Listing = Listing{By: l.By, VendorOrg: l.VendorOrg, Proof: l.Proof}
	if ad, ok := catalog.Lock().Admission[t.ID]; ok {
		e.Admission = &ad
	}
	rec, verified := catalog.VerifiedFor(t)
	if verified {
		e.Verification = &Verification{Tag: rec.Tag, Digest: rec.Digest, HTTPStatus: rec.HTTPStatus, Arches: append([]string{}, rec.Arches...), RunURL: rec.RunURL}
	}
	f, err := compose.Parse([]byte(t.Compose))
	if err != nil {
		return e, fmt.Errorf("parse %s: %w", t.ID, err)
	}
	for name, svc := range f.Services {
		s := Service{Name: name, Image: svc.Image, Ports: []int{}, Volumes: []string{}}
		for _, p := range svc.Ports {
			s.Ports = append(s.Ports, p.ContainerPort)
		}
		for _, v := range svc.Volumes {
			src := v.Name
			if src == "" {
				src = "host path"
			}
			s.Volumes = append(s.Volumes, src+" -> "+v.ContainerPath)
		}
		e.Services = append(e.Services, s)
		e.Images = append(e.Images, imageFor(name, svc.Image, e.SourceRepo))
	}
	sort.Slice(e.Images, func(i, j int) bool { return e.Images[i].Service < e.Images[j].Service })
	e.Arch = archStates(e.Images, rec, verified)
	e.Trust = trustFor(e, rec, verified)
	sort.Slice(e.Services, func(i, j int) bool { return e.Services[i].Name < e.Services[j].Name })

	f2, err := compose.Parse([]byte(t.Compose))
	if err != nil {
		return e, fmt.Errorf("parse %s: %w", t.ID, err)
	}
	secretEnv, unresolved, err := compose.ResolveMagicVars(f2, detectGen, detectPersist)
	if err != nil {
		return e, fmt.Errorf("resolve %s: %w", t.ID, err)
	}
	kinds := map[string]string{}
	for svc, keys := range secretEnv {
		for _, k := range keys {
			kinds[svc+"\x00"+k] = kindGenerated
		}
	}
	for _, u := range unresolved {
		kinds[u.Service+"\x00"+u.EnvKey] = kindRequired
		e.NeedsConfig = true
	}
	for name, svc := range f.Services {
		for key := range svc.Environment {
			kind := kinds[name+"\x00"+key]
			if kind == "" {
				kind = kindPreset
			}
			e.Env = append(e.Env, EnvVar{Service: name, Key: key, Kind: kind})
		}
	}
	sort.Slice(e.Env, func(i, j int) bool {
		if e.Env[i].Service != e.Env[j].Service {
			return e.Env[i].Service < e.Env[j].Service
		}
		return e.Env[i].Key < e.Env[j].Key
	})
	return e, nil
}

// archesOf keeps the linux arches the catalog tracks. Empty means the
// registry scan has not checked the template, never a claim.
func archesOf(platforms []string) []string {
	var out []string
	for _, a := range []string{"amd64", "arm64"} {
		for _, p := range platforms {
			if p == "linux/"+a {
				out = append(out, a)
			}
		}
	}
	return out
}

// applyMeta fills the marketplace fields of a Go-literal template from the
// recorded metadata, never overriding text the template already carries.
func applyMeta(e *Entry, m catalog.TemplateMeta) {
	e.SourceRepo = m.SourceRepo
	if m.License != "" {
		e.License = &License{SPDX: m.License, Class: m.LicenseClass, URL: m.LicenseURL}
	}
	if e.Description == "" {
		e.Description = sanitize(m.Description)
	}
}

func applyManifest(e *Entry, m *catalog.Manifest) {
	e.SourceRepo = m.SourceRepo
	if m.License != "" {
		e.License = &License{SPDX: m.License, Class: m.LicenseClass, URL: m.LicenseURL}
	}
	if m.HostedUse != "" {
		e.HostedUse = m.HostedUse
	}
	if m.Status != "" {
		e.Status, e.StatusReason, e.ReplacedBy = m.Status, sanitize(m.StatusReason), m.ReplacedBy
	}
	if len(m.Aliases) > 0 {
		e.Aliases = append([]string{}, m.Aliases...)
	}
	for _, s := range m.FirstRun {
		e.FirstRun = append(e.FirstRun, sanitize(s))
	}
}

var platformsByRef = func() map[string][]string {
	out := map[string][]string{}
	for _, p := range catalog.PlatformsData().Images {
		out[p.Ref()] = p.Platforms
	}
	return out
}()

func imageFor(service, ref, sourceRepo string) Image {
	img, tag := catalog.SplitImageRef(ref)
	im := Image{Service: service, Image: img, Tag: tag, SourceClass: catalog.ImageSourceClass(ref, sourceRepo), Platforms: platformsByRef[ref]}
	if l, ok := catalog.Lock().Images[ref]; ok {
		im.Digest, im.Signed, im.Provenance, im.SBOM, im.NonRoot = l.Digest, l.Signed, l.Provenance, l.SBOM, l.NonRoot
	}
	return im
}

// archStates reports tested (a recorded passing run on that arch), published
// (every image lists it), absent (the scan found a missing image) or unknown.
func archStates(images []Image, rec catalog.VerifiedRecord, verified bool) map[string]string {
	out := map[string]string{}
	for _, a := range []string{"amd64", "arm64"} {
		state := "published"
		for _, im := range images {
			switch {
			case len(im.Platforms) == 0:
				state = "unknown"
			case !hasPlatform(im.Platforms, "linux/"+a) && state != "unknown":
				state = "absent"
			}
		}
		if len(images) == 0 {
			state = "unknown"
		}
		if verified {
			for _, ra := range rec.Arches {
				if ra == a {
					state = "tested"
				}
			}
		}
		out[a] = state
	}
	return out
}

func hasPlatform(list []string, p string) bool {
	for _, v := range list {
		if v == p {
			return true
		}
	}
	return false
}

// trustFor: verified needs a recorded smoke pass on the pinned images; the
// tested architectures are disclosed in verification.arches. Signed adds a
// cosign identity and provenance on every upstream image.
func trustFor(e Entry, rec catalog.VerifiedRecord, verified bool) Trust {
	tr := Trust{Level: trustListed}
	if e.Admission != nil {
		tr.EvidenceURL = e.Admission.RunURL
	}
	if !verified || len(rec.Arches) == 0 {
		return tr
	}
	// Without a CI run link the public status page is the evidence of record.
	evidence := rec.RunURL
	if evidence == "" {
		evidence = statusPageURL
	}
	tr = Trust{Level: trustVerified, EvidenceURL: evidence}
	upstream := 0
	for _, im := range e.Images {
		if im.SourceClass != catalog.SourceUpstream {
			continue
		}
		upstream++
		if im.Signed == nil || !im.Provenance {
			return tr
		}
	}
	if upstream == 0 {
		return tr
	}
	tr.Level = trustVerifiedSigned
	return tr
}

func generate(templates []catalog.Template) ([]byte, error) {
	out := make([]Entry, 0, len(templates))
	for _, t := range templates {
		e, err := buildEntry(t)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", " ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(Index{SchemaVersion: SchemaVersion, Templates: out, Tombstones: catalog.Tombstones()}); err != nil {
		return nil, fmt.Errorf("encode gallery data: %w", err)
	}
	return buf.Bytes(), nil
}

func main() {
	path := flag.String("out", "scripts/gen-template-gallery/catalog-index.json", "JSON file to write")
	check := flag.Bool("check", false, "fail instead of writing when the file is stale")
	flag.Parse()

	data, err := generate(catalog.Templates)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *check {
		cur, _ := os.ReadFile(*path)
		if !bytes.Equal(cur, data) {
			fmt.Fprintf(os.Stderr, "%s is stale: run go run ./scripts/gen-template-gallery\n", *path)
			os.Exit(1)
		}
		return
	}
	if err := os.WriteFile(*path, data, 0o644); err != nil { //nolint:gosec // docs data, world-readable by design
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
