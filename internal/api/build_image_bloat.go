package api

import (
	"context"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// bloatLayerLookups bounds how many flagged builds one response inspects, since each reads two image histories.
const bloatLayerLookups = 20

const bytesPerMiB = int64(1) << 20

// imageBloat is the warning on a build whose image grew a lot over the app's previous build.
type imageBloat struct {
	PreviousBytes int64   `json:"previous_bytes"`
	GrowthPercent float64 `json:"growth_percent"`
	// Layer is the build step that grew most, when both images are still on the node.
	Layer      string `json:"layer,omitempty"`
	LayerBytes int64  `json:"layer_bytes,omitempty"`

	tag, prevTag string
}

// bloatPolicy is the live threshold pair; a zero percent turns the warning off.
type bloatPolicy struct {
	Percent  int
	MinBytes int64
}

func (rt *Router) bloatPolicy(ctx context.Context) bloatPolicy {
	if rt.runtimeSettings == nil {
		return bloatPolicy{}
	}
	return bloatPolicy{
		Percent:  rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildImageBloatPercent),
		MinBytes: int64(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyBuildImageBloatMinMB)) * bytesPerMiB,
	}
}

func (p bloatPolicy) check(ch imageChange, tag string) *imageBloat {
	if p.Percent <= 0 || ch.PrevBytes <= 0 || ch.Delta <= 0 || ch.Delta < p.MinBytes {
		return nil
	}
	pct := float64(ch.Delta) / float64(ch.PrevBytes) * 100
	if pct < float64(p.Percent) {
		return nil
	}
	return &imageBloat{PreviousBytes: ch.PrevBytes, GrowthPercent: pct, tag: tag, prevTag: ch.PrevTag}
}

// nameGrownLayers fills the grown layer of flagged rows from the local image
// history; a missing image leaves the warning without a layer name.
func (rt *Router) nameGrownLayers(ctx context.Context, rows []buildRow) {
	if rt.imageInventory == nil {
		return
	}
	inv, err := rt.imageInventory("")
	if err != nil || inv == nil {
		return
	}
	left := bloatLayerLookups
	for i := range rows {
		b := rows[i].ImageBloat
		if b == nil || left == 0 {
			continue
		}
		left--
		cur, err := inv.ImageLayerHistory(ctx, b.tag)
		if err != nil || len(cur) == 0 {
			continue
		}
		prev, err := inv.ImageLayerHistory(ctx, b.prevTag)
		if err != nil || len(prev) == 0 {
			continue
		}
		b.Layer, b.LayerBytes = grownLayer(prev, cur)
	}
}

// grownLayer returns the build step whose total size grew most from prev to cur.
// Steps match by their command text, so a step that is new counts in full.
func grownLayer(prev, cur []docker.ImageLayer) (string, int64) {
	before := map[string]int64{}
	for _, l := range prev {
		before[l.CreatedBy] += l.SizeBytes
	}
	after := map[string]int64{}
	for _, l := range cur {
		after[l.CreatedBy] += l.SizeBytes
	}
	names := make([]string, 0, len(after))
	for n := range after {
		names = append(names, n)
	}
	sort.Strings(names)
	var best string
	var bestGrowth int64
	for _, n := range names {
		if g := after[n] - before[n]; g > bestGrowth {
			best, bestGrowth = n, g
		}
	}
	return layerLabel(best), bestGrowth
}

// layerLabel keeps the readable part of a history command and bounds its length.
func layerLabel(cmd string) string {
	cmd = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cmd), "/bin/sh -c #(nop)"))
	cmd = strings.TrimSpace(strings.TrimPrefix(cmd, "/bin/sh -c"))
	cmd = strings.Join(strings.Fields(cmd), " ")
	if r := []rune(cmd); len(r) > 120 {
		cmd = string(r[:120])
	}
	return cmd
}
