// Command gen-arm-data writes docs/public/arm64-compat.json, a per template and
// per image view of which architectures the registry manifests list. It reads
// only the checked-in scan (internal/catalog/platforms.json), so it never
// touches the network. Refresh the scan first with scripts/scan-catalog-platforms.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const (
	outPath          = "docs/public/arm64-compat.json"
	staleDaysEnv     = "ARM_DATA_STALE_DAYS"
	defaultStaleDays = 14
)

// Per-image and per-template verdict values.
const (
	yes     = "yes"
	no      = "no"
	unknown = "unknown"
)

// Source says where an image verdict comes from: a manifest read in the latest
// scan, an older cached read, or nothing at all.
const (
	srcManifest = "manifest"
	srcCache    = "cache"
	srcUnknown  = "unknown"
)

type imageRow struct {
	Ref       string   `json:"ref"`
	Platforms []string `json:"platforms"`
	AMD64     string   `json:"amd64"`
	ARM64     string   `json:"arm64"`
	Source    string   `json:"source"`
	CheckedAt string   `json:"checked_at,omitempty"`
}

type templateRow struct {
	ID     string     `json:"id"`
	AMD64  string     `json:"amd64"`
	ARM64  string     `json:"arm64"`
	Images []imageRow `json:"images"`
}

type counts struct {
	Templates     int `json:"templates"`
	Images        int `json:"images"`
	ARM64Yes      int `json:"arm64_yes"`
	ARM64No       int `json:"arm64_no"`
	ARM64Unknown  int `json:"arm64_unknown"`
	ImageARM64Yes int `json:"image_arm64_yes"`
	ImageARM64No  int `json:"image_arm64_no"`
	ImageUnknown  int `json:"image_arm64_unknown"`
}

type document struct {
	GeneratedAt string        `json:"generated_at"`
	Note        string        `json:"note"`
	Counts      counts        `json:"counts"`
	Templates   []templateRow `json:"templates"`
}

func hasPlatform(list []string, os, arch string) bool {
	for _, p := range list {
		if p == os+"/"+arch {
			return true
		}
	}
	return false
}

func verdict(known, listed bool) string {
	switch {
	case !known:
		return unknown
	case listed:
		return yes
	}
	return no
}

// imageFor never claims an arch the manifest does not list: no scan entry, a
// scan error or an empty platform list all stay unknown.
func imageFor(ref string, byRef map[string]catalog.ImagePlatforms, scannedAt time.Time, stale time.Duration) imageRow {
	row := imageRow{Ref: ref, Platforms: []string{}, AMD64: unknown, ARM64: unknown, Source: srcUnknown}
	p, ok := byRef[ref]
	if !ok || p.Error != "" || len(p.Platforms) == 0 {
		return row
	}
	row.Platforms = append([]string{}, p.Platforms...)
	sort.Strings(row.Platforms)
	row.AMD64 = verdict(true, hasPlatform(p.Platforms, "linux", "amd64"))
	row.ARM64 = verdict(true, hasPlatform(p.Platforms, "linux", "arm64"))
	row.CheckedAt = p.CheckedAt
	row.Source = srcManifest
	if t, err := time.Parse(time.RFC3339, p.CheckedAt); err != nil || scannedAt.Sub(t) > stale {
		row.Source = srcCache
	}
	return row
}

// combine is "yes" only when every image says yes, "no" when any says no, and
// unknown otherwise, so one unreadable image never produces a claim.
func combine(rows []imageRow, pick func(imageRow) string, complete bool) string {
	if len(rows) == 0 || !complete {
		for _, r := range rows {
			if pick(r) == no {
				return no
			}
		}
		return unknown
	}
	out := yes
	for _, r := range rows {
		switch pick(r) {
		case no:
			return no
		case unknown:
			out = unknown
		}
	}
	return out
}

func build(templates []catalog.Template, plat catalog.PlatformsFile, stale time.Duration) document {
	byRef := map[string]catalog.ImagePlatforms{}
	for _, p := range plat.Images {
		byRef[p.Ref()] = p
	}
	scannedAt, _ := time.Parse(time.RFC3339, plat.ScannedAt)
	ts := append([]catalog.Template(nil), templates...)
	sort.Slice(ts, func(i, j int) bool { return ts[i].ID < ts[j].ID })

	doc := document{
		GeneratedAt: plat.ScannedAt,
		Note:        "yes means every image manifest lists the architecture. It is a manifest fact, not a tested claim. unknown means no readable manifest or an install-time image tag.",
		Templates:   []templateRow{},
	}
	seen := map[string]bool{}
	for _, t := range ts {
		refs, complete := catalog.ComposeImages(t.Compose)
		row := templateRow{ID: t.ID, Images: []imageRow{}}
		for _, ref := range refs {
			img := imageFor(ref, byRef, scannedAt, stale)
			row.Images = append(row.Images, img)
			if !seen[ref] {
				seen[ref] = true
				doc.Counts.Images++
				switch img.ARM64 {
				case yes:
					doc.Counts.ImageARM64Yes++
				case no:
					doc.Counts.ImageARM64No++
				default:
					doc.Counts.ImageUnknown++
				}
			}
		}
		row.AMD64 = combine(row.Images, func(r imageRow) string { return r.AMD64 }, complete)
		row.ARM64 = combine(row.Images, func(r imageRow) string { return r.ARM64 }, complete)
		switch row.ARM64 {
		case yes:
			doc.Counts.ARM64Yes++
		case no:
			doc.Counts.ARM64No++
		default:
			doc.Counts.ARM64Unknown++
		}
		doc.Templates = append(doc.Templates, row)
	}
	doc.Counts.Templates = len(doc.Templates)
	return doc
}

func staleWindow() time.Duration {
	days := defaultStaleDays
	if v, err := strconv.Atoi(os.Getenv(staleDaysEnv)); err == nil && v > 0 {
		days = v
	}
	return time.Duration(days) * 24 * time.Hour
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	check := flag.Bool("check", false, "fail if the file is stale")
	flag.Parse()
	body, err := json.MarshalIndent(build(catalog.Templates, catalog.PlatformsData(), staleWindow()), "", "  ")
	if err != nil {
		fail(err)
	}
	body = append(body, '\n')
	old, _ := os.ReadFile(outPath)
	if string(old) == string(body) {
		return
	}
	if *check {
		fail(fmt.Errorf("%s is stale; run go run ./scripts/gen-arm-data", outPath))
	}
	if err := os.WriteFile(outPath, body, 0o600); err != nil { //nolint:gosec // fixed repo doc path
		fail(err)
	}
}
