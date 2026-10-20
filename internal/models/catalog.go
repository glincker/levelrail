package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

//go:embed catalog_local.json
var embeddedCatalog []byte

// Tiers of the curated local model list.
const (
	TierLight   = "light"
	TierDefault = "default"
	TierPlus    = "plus"

	KindChat      = "chat"
	KindEmbedding = "embedding"
)

// curatedLicenses are the only SPDX identifiers the curated list may carry.
var curatedLicenses = []string{"Apache-2.0", "MIT"}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CatalogEntry is one curated local model. Weights are never shipped by
// Levelrail: the user's server downloads them from Hugging Face on the
// user's action.
type CatalogEntry struct {
	ID            string  `json:"id"`
	Label         string  `json:"label"`
	Vendor        string  `json:"vendor"`
	Repo          string  `json:"repo"`
	File          string  `json:"file"`
	ParamsB       float64 `json:"params_b"`
	Quant         string  `json:"quant"`
	DownloadBytes int64   `json:"download_bytes"`
	SHA256        string  `json:"sha256"`
	MinRAMMiB     int     `json:"min_ram_mib"`
	ContextLength int     `json:"context_length"`
	License       string  `json:"license"`
	Tier          string  `json:"tier"`
	Kind          string  `json:"kind"`
	ToolCalling   bool    `json:"tool_calling"`
	Notes         string  `json:"notes"`
}

// Ref is the model reference stored on a resource for this entry.
func (e CatalogEntry) Ref() string { return e.Repo + ":" + e.Quant }

// Catalog is the curated list with its provenance.
type Catalog struct {
	Version    int            `json:"version"`
	VerifiedOn string         `json:"verified_on"`
	Note       string         `json:"note"`
	Models     []CatalogEntry `json:"models"`
}

// ParseCatalog validates and returns a curated list from JSON.
func ParseCatalog(data []byte) (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return Catalog{}, fmt.Errorf("models: parse local catalog: %w", err)
	}
	seen := map[string]bool{}
	for i, e := range c.Models {
		switch {
		case e.ID == "" || !nameRe.MatchString(strings.ReplaceAll(e.ID, ".", "-")):
			return Catalog{}, fmt.Errorf("models: catalog entry %d has an invalid id %q", i, e.ID)
		case seen[e.ID]:
			return Catalog{}, fmt.Errorf("models: catalog id %q is duplicated", e.ID)
		case !hfRepoRe.MatchString(e.Repo) || e.File == "" || strings.ContainsAny(e.File, "/\\") || !strings.HasSuffix(e.File, ".gguf"):
			return Catalog{}, fmt.Errorf("models: catalog entry %q has an invalid repo or file", e.ID)
		case !slices.Contains(curatedLicenses, e.License):
			return Catalog{}, fmt.Errorf("models: catalog entry %q has licence %q; only %v may be curated", e.ID, e.License, curatedLicenses)
		case e.Tier != TierLight && e.Tier != TierDefault && e.Tier != TierPlus:
			return Catalog{}, fmt.Errorf("models: catalog entry %q has an unknown tier %q", e.ID, e.Tier)
		case e.Kind != KindChat && e.Kind != KindEmbedding:
			return Catalog{}, fmt.Errorf("models: catalog entry %q has an unknown kind %q", e.ID, e.Kind)
		case e.DownloadBytes <= 0 || e.MinRAMMiB <= 0 || e.Quant == "":
			return Catalog{}, fmt.Errorf("models: catalog entry %q is missing a size, memory need or quantization", e.ID)
		case e.SHA256 != "" && !sha256Re.MatchString(e.SHA256):
			return Catalog{}, fmt.Errorf("models: catalog entry %q has a malformed sha256", e.ID)
		}
		seen[e.ID] = true
	}
	if len(c.Models) == 0 {
		return Catalog{}, fmt.Errorf("models: the local catalog is empty")
	}
	return c, nil
}

var (
	catalogOnce sync.Once
	catalogVal  Catalog
	catalogErr  error
)

// LocalCatalog returns the embedded curated list.
func LocalCatalog() (Catalog, error) {
	catalogOnce.Do(func() { catalogVal, catalogErr = ParseCatalog(embeddedCatalog) })
	return catalogVal, catalogErr
}

// Find returns the entry with id.
func (c Catalog) Find(id string) (CatalogEntry, bool) {
	for _, e := range c.Models {
		if e.ID == id {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// FindWeights returns the entry serving repo and file.
func (c Catalog) FindWeights(repo, file string) (CatalogEntry, bool) {
	for _, e := range c.Models {
		if strings.EqualFold(e.Repo, repo) && e.File == file {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// FindRepo returns the entry for repo and an optional quant suffix.
func (c Catalog) FindRepo(repo, quant string) (CatalogEntry, bool) {
	for _, e := range c.Models {
		if strings.EqualFold(e.Repo, repo) && (quant == "" || strings.EqualFold(e.Quant, quant)) {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// Sorted returns the entries ordered by tier then size.
func (c Catalog) Sorted() []CatalogEntry {
	out := slices.Clone(c.Models)
	rank := map[string]int{TierLight: 0, TierDefault: 1, TierPlus: 2}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Tier] != rank[out[j].Tier] {
			return rank[out[i].Tier] < rank[out[j].Tier]
		}
		return out[i].DownloadBytes < out[j].DownloadBytes
	})
	return out
}

// LicenseNotice is the text shown for a repository that is not curated.
func LicenseNotice(repoLicense string) string {
	l := strings.TrimSpace(repoLicense)
	if l == "" {
		l = "unknown"
	}
	return fmt.Sprintf("This repository is not on the curated list. Its licence is %q and was not reviewed by Levelrail. "+
		"Your server downloads the weights directly from Hugging Face; you are responsible for complying with the licence.", l)
}
