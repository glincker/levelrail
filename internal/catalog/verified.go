package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed verified.json
var verifiedJSON []byte

// VerifiedRecord is one passed smoke run. Images lists every pinned ref the
// run used, so a record stops counting the moment any of them changes.
type VerifiedRecord struct {
	Tag    string   `json:"tag"`
	Images []string `json:"images"`
	// Digest is the primary image digest the run pulled; empty for historical local runs.
	Digest     string   `json:"digest,omitempty"`
	Arches     []string `json:"arches"`
	HTTPStatus int      `json:"http_status,omitempty"`
	// RunURL links the CI run that produced the record; empty for historical local runs.
	RunURL string `json:"run_url,omitempty"`
}

var verifiedData = loadVerified()

func loadVerified() map[string]VerifiedRecord {
	m := map[string]VerifiedRecord{}
	if err := json.Unmarshal(verifiedJSON, &m); err != nil {
		panic("catalog: verified.json: " + err.Error())
	}
	return m
}

// VerifiedData returns the committed smoke records, keyed by template id.
func VerifiedData() map[string]VerifiedRecord { return verifiedData }

// PinnedImages returns the template's distinct image refs, sorted. ok is
// false when any ref is unpinned (no tag, latest, digest or a variable).
func PinnedImages(t Template) (refs []string, ok bool) {
	refs, ok = ComposeImages(t.Compose)
	if !ok {
		return nil, false
	}
	for _, r := range refs {
		if _, tag := SplitImageRef(r); tag == "" || tag == "latest" {
			return nil, false
		}
	}
	return refs, true
}

// PrimaryTag is the tag of the template's main image: the service its
// healthcheck names, else the first ref in sorted order.
func PrimaryTag(t Template) string {
	refs, ok := PinnedImages(t)
	if !ok || len(refs) == 0 {
		return ""
	}
	ref := refs[0]
	if t.Manifest != nil && t.Manifest.Healthcheck != nil {
		if img := serviceImage(t.Compose, t.Manifest.Healthcheck.Service); img != "" {
			ref = img
		}
	}
	_, tag := SplitImageRef(ref)
	return tag
}

func serviceImage(composeBody, service string) string {
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(composeBody), &doc); err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Services[service].Image)
}

// VerifiedFor returns the record only while it still matches the template's
// current pinned images. Verified is a fact about those tags, never a date.
func VerifiedFor(t Template) (VerifiedRecord, bool) {
	rec, ok := verifiedData[t.ID]
	if !ok {
		return VerifiedRecord{}, false
	}
	refs, ok := PinnedImages(t)
	if !ok || strings.Join(refs, "\n") != strings.Join(rec.Images, "\n") {
		return VerifiedRecord{}, false
	}
	// A recorded digest must still match the lock, else the tag was repushed.
	if l, ok := lockData.Images[refs[0]]; ok && rec.Digest != "" && l.Digest != "" && rec.Digest != l.Digest {
		return VerifiedRecord{}, false
	}
	return rec, true
}

// IsVerified reports whether the catalog template id has a smoke record that
// still matches its pinned images.
func IsVerified(id string) bool {
	t, ok := TemplateByID(id)
	if !ok {
		return false
	}
	_, ok = VerifiedFor(t)
	return ok
}

// VerifiedRun is one passed smoke run to fold into verified.json.
type VerifiedRun struct {
	Arch       string
	Digest     string
	HTTPStatus int
	RunURL     string
}

// RecordVerified merges one passed run for t into the file at path.
// Arches already recorded for the same pinned images are kept.
func RecordVerified(path string, t Template, run VerifiedRun) error {
	refs, ok := PinnedImages(t)
	if !ok {
		return fmt.Errorf("template %s has unpinned images and cannot be verified", t.ID)
	}
	m := map[string]VerifiedRecord{}
	if raw, err := os.ReadFile(path); err == nil { //nolint:gosec // path is a dev-tool flag
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
	rec := VerifiedRecord{Tag: PrimaryTag(t), Images: refs, Digest: run.Digest, HTTPStatus: run.HTTPStatus, RunURL: run.RunURL}
	if old, ok := m[t.ID]; ok && strings.Join(old.Images, "\n") == strings.Join(refs, "\n") {
		rec.Arches = old.Arches
	}
	if run.Arch != "" && !containsString(rec.Arches, run.Arch) {
		rec.Arches = append(rec.Arches, run.Arch)
	}
	sort.Strings(rec.Arches)
	if rec.Arches == nil {
		rec.Arches = []string{}
	}
	m[t.ID] = rec
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644) //nolint:gosec // committed data, world-readable by design
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
