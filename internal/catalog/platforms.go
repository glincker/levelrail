package catalog

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed platforms.json
var platformsJSON []byte

// ImagePlatforms is one scanned image reference and the platforms its
// registry manifest lists. An empty list with an Error means unknown.
type ImagePlatforms struct {
	Image     string   `json:"image"`
	Tag       string   `json:"tag"`
	Platforms []string `json:"platforms"`
	CheckedAt string   `json:"checked_at"`
	Error     string   `json:"error,omitempty"`
}

// Ref returns the pullable reference the scan inspected.
func (p ImagePlatforms) Ref() string {
	if p.Tag == "" {
		return p.Image
	}
	return p.Image + ":" + p.Tag
}

// PlatformsFile is the checked-in output of scripts/scan-catalog-platforms.
type PlatformsFile struct {
	ScannedAt string              `json:"scanned_at"`
	Images    []ImagePlatforms    `json:"images"`
	Templates map[string][]string `json:"templates"`
}

var platformsData = loadPlatforms()

func loadPlatforms() PlatformsFile {
	var f PlatformsFile
	if err := json.Unmarshal(platformsJSON, &f); err != nil {
		panic("catalog: platforms.json: " + err.Error())
	}
	return f
}

// PlatformsData returns the committed scan result.
func PlatformsData() PlatformsFile { return platformsData }

// Merging at init keeps Platforms out of every template literal: empty
// means "not checked", never a claim.
func init() {
	for i := range Templates {
		if p := platformsData.Templates[Templates[i].ID]; len(p) > 0 {
			Templates[i].Platforms = append([]string(nil), p...)
		}
	}
}

// ComposeImages returns the distinct image references of a compose body.
// ok is false when any service has no literal image (build only or a
// ${VAR} reference), so callers must treat the template as unknown.
func ComposeImages(compose string) (images []string, ok bool) {
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil || len(doc.Services) == 0 {
		return nil, false
	}
	ok = true
	seen := map[string]bool{}
	for _, svc := range doc.Services {
		img := strings.TrimSpace(svc.Image)
		if img == "" || strings.Contains(img, "$") {
			ok = false
			continue
		}
		if !seen[img] {
			seen[img] = true
			images = append(images, img)
		}
	}
	sort.Strings(images)
	return images, ok
}

// SplitImageRef splits repo and tag. A digest-pinned ref keeps the whole
// string as the image and an empty tag.
func SplitImageRef(ref string) (image, tag string) {
	if strings.Contains(ref, "@") {
		return ref, ""
	}
	slash := strings.LastIndex(ref, "/")
	if colon := strings.LastIndex(ref, ":"); colon > slash {
		return ref[:colon], ref[colon+1:]
	}
	return ref, "latest"
}
