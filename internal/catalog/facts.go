package catalog

import (
	"sort"
	"sync"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Trust levels. Every entry is at least listed.
const (
	TrustListed         = "listed"
	TrustVerified       = "verified"
	TrustVerifiedSigned = "verified_signed"
)

// StatusPageURL records smoke passes that predate CI run links.
const StatusPageURL = "https://github.com/glincker/levelrail/blob/main/docs/templates-status.md"

// License is the stated license and its class.
type License struct {
	SPDX  string `json:"spdx"`
	Class string `json:"class"`
	URL   string `json:"url,omitempty"`
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

// ImageFact is one service image with the facts tied to its pinned tag.
type ImageFact struct {
	Service     string    `json:"service"`
	Image       string    `json:"image"`
	Tag         string    `json:"tag"`
	Digest      string    `json:"digest,omitempty"`
	SourceClass string    `json:"source_class"`
	Platforms   []string  `json:"platforms,omitempty"`
	Signed      *SignedBy `json:"signed"`
	Provenance  bool      `json:"provenance"`
	SBOM        bool      `json:"sbom"`
	NonRoot     bool      `json:"non_root"`
}

// Marketplace is every derived marketplace fact for one template. The site
// index and the product API both read it, so the two never disagree.
type Marketplace struct {
	SourceRepo   string
	License      *License
	Status       string
	StatusReason string
	ReplacedBy   string
	// HostedUse feeds the public index only; the product API never exposes it.
	HostedUse    string
	Aliases      []string
	Listing      Listing
	Trust        Trust
	Arch         map[string]string
	Verification *Verification
	Admission    *Admission
	Images       []ImageFact
}

// MarketplaceFor derives the marketplace facts of one template. A compose body
// that fails to parse yields the facts without images, never an error.
func MarketplaceFor(t Template) Marketplace {
	m := Marketplace{Status: StatusActive, HostedUse: "unknown", Trust: Trust{Level: TrustListed}, Aliases: []string{}, Images: []ImageFact{}}
	if mf := t.Manifest; mf != nil {
		m.SourceRepo = mf.SourceRepo
		if mf.License != "" {
			m.License = &License{SPDX: mf.License, Class: mf.LicenseClass, URL: mf.LicenseURL}
		}
		if mf.HostedUse != "" {
			m.HostedUse = mf.HostedUse
		}
		if mf.Status != "" {
			m.Status, m.StatusReason, m.ReplacedBy = mf.Status, mf.StatusReason, mf.ReplacedBy
		}
		if len(mf.Aliases) > 0 {
			m.Aliases = append([]string{}, mf.Aliases...)
		}
	} else if meta, ok := MetaFor(t.ID); ok {
		m.SourceRepo = meta.SourceRepo
		if meta.License != "" {
			m.License = &License{SPDX: meta.License, Class: meta.LicenseClass, URL: meta.LicenseURL}
		}
		if meta.Status != "" {
			m.Status, m.StatusReason, m.ReplacedBy = meta.Status, meta.StatusReason, meta.ReplacedBy
		}
	}
	m.Listing = ListingFor(t)
	if ad, ok := Lock().Admission[t.ID]; ok {
		m.Admission = &ad
	}
	rec, verified := VerifiedFor(t)
	if verified {
		m.Verification = &Verification{Tag: rec.Tag, Digest: rec.Digest, HTTPStatus: rec.HTTPStatus, Arches: append([]string{}, rec.Arches...), RunURL: rec.RunURL}
	}
	if f, err := compose.Parse([]byte(t.Compose)); err == nil {
		for name, svc := range f.Services {
			m.Images = append(m.Images, imageFact(name, svc.Image, m.SourceRepo))
		}
		sort.Slice(m.Images, func(i, j int) bool { return m.Images[i].Service < m.Images[j].Service })
	}
	m.Arch = archStates(m.Images, rec, verified)
	m.Trust = trustFor(m, rec, verified)
	return m
}

var (
	marketplaceOnce  sync.Once
	marketplaceCache map[string]Marketplace
)

// MarketplaceByID is the cached form for request paths: the catalog is static,
// so facts are derived once.
func MarketplaceByID(id string) (Marketplace, bool) {
	marketplaceOnce.Do(func() {
		marketplaceCache = make(map[string]Marketplace, len(Templates))
		for _, t := range Templates {
			marketplaceCache[t.ID] = MarketplaceFor(t)
		}
	})
	m, ok := marketplaceCache[id]
	return m, ok
}

var platformsByRef = sync.OnceValue(func() map[string][]string {
	out := map[string][]string{}
	for _, p := range PlatformsData().Images {
		out[p.Ref()] = p.Platforms
	}
	return out
})

func imageFact(service, ref, sourceRepo string) ImageFact {
	img, tag := SplitImageRef(ref)
	im := ImageFact{Service: service, Image: img, Tag: tag, SourceClass: ImageSourceClass(ref, sourceRepo), Platforms: platformsByRef()[ref]}
	if l, ok := Lock().Images[ref]; ok {
		im.Digest, im.Signed, im.Provenance, im.SBOM, im.NonRoot = l.Digest, l.Signed, l.Provenance, l.SBOM, l.NonRoot
	}
	return im
}

// archStates reports tested (a recorded passing run on that arch), published
// (every image lists it), absent (the scan found a missing image) or unknown.
func archStates(images []ImageFact, rec VerifiedRecord, verified bool) map[string]string {
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
func trustFor(m Marketplace, rec VerifiedRecord, verified bool) Trust {
	tr := Trust{Level: TrustListed}
	if m.Admission != nil {
		tr.EvidenceURL = m.Admission.RunURL
	}
	if !verified || len(rec.Arches) == 0 {
		return tr
	}
	// Without a CI run link the public status page is the evidence of record.
	evidence := rec.RunURL
	if evidence == "" {
		evidence = StatusPageURL
	}
	tr = Trust{Level: TrustVerified, EvidenceURL: evidence}
	upstream := 0
	for _, im := range m.Images {
		if im.SourceClass != SourceUpstream {
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
	tr.Level = TrustVerifiedSigned
	return tr
}
