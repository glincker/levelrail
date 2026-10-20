package nodecapacity

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/janitor"
)

// Cleanup classes.
const (
	ClassDanglingImages    = "dangling_images"
	ClassBuildCache        = "build_cache"
	ClassStoppedContainers = "stopped_containers"
	ClassRollbackImages    = "rollback_images"
	ClassOrphanedVolumes   = "orphaned_volumes"
)

// Class is one group of cleanup candidates. Deletable false means the items are shown only.
type Class struct {
	Name      string         `json:"name"`
	Deletable bool           `json:"deletable"`
	Bytes     int64          `json:"bytes"`
	Items     []janitor.Item `json:"items"`
}

// Preview is a cleanup grouped by class.
type Preview struct {
	Classes    []Class `json:"classes"`
	TotalBytes int64   `json:"total_bytes"`
}

// Result is an executed cleanup.
type Result struct {
	Preview
	FreedBytes int64  `json:"freed_bytes"`
	Error      string `json:"error,omitempty"`
}

// SafeStages are the only stages a guardrail cleanup ever runs: volumes and deploy attempts are excluded.
var SafeStages = []janitor.Stage{janitor.StageContainers, janitor.StageImages, janitor.StageBuildCache}

var previewStages = append(append([]janitor.Stage{}, SafeStages...), janitor.StageVolumes)

// Runner is the janitor surface a cleanup needs. *janitor.Engine satisfies it.
type Runner interface {
	Run(ctx context.Context, opts janitor.Options) (janitor.Report, error)
}

// ImageSource lists local images so tagged unused ones can be shown as kept.
type ImageSource interface {
	ListAllImages(ctx context.Context) ([]docker.ImageRecord, error)
}

// BuildPreview runs a dry pass and groups the outcome. Nothing is removed.
func BuildPreview(ctx context.Context, run Runner, imgs ImageSource) (Preview, error) {
	rep, err := run.Run(ctx, janitor.Options{Trigger: janitor.TriggerNodeGuardrail, DryRun: true, Force: true, Stages: previewStages})
	if err != nil {
		return Preview{}, fmt.Errorf("nodecapacity: preview: %w", err)
	}
	p := group(rep, false)
	if imgs != nil {
		recs, lerr := imgs.ListAllImages(ctx)
		if lerr != nil {
			return Preview{}, fmt.Errorf("nodecapacity: list images: %w", lerr)
		}
		p.add(Class{Name: ClassRollbackImages, Items: keptImages(recs)})
	}
	return p, nil
}

// Execute removes the safe classes only and reports what was freed.
func Execute(ctx context.Context, run Runner, trigger string) (Result, error) {
	rep, err := run.Run(ctx, janitor.Options{Trigger: trigger, Force: true, Stages: SafeStages})
	if err != nil {
		return Result{}, fmt.Errorf("nodecapacity: cleanup: %w", err)
	}
	return Result{Preview: group(rep, true), FreedBytes: rep.ReclaimedBytes, Error: rep.Error}, nil
}

func keptImages(recs []docker.ImageRecord) []janitor.Item {
	items := []janitor.Item{}
	for _, r := range recs {
		if len(r.Tags) == 0 || r.Containers > 0 {
			continue
		}
		items = append(items, janitor.Item{Name: r.Tags[0], Detail: "kept for rollback", SizeBytes: r.SizeBytes, Action: janitor.ActionKept})
	}
	return items
}

func (p *Preview) add(c Class) {
	for _, it := range c.Items {
		c.Bytes += max(it.SizeBytes, 0)
	}
	if c.Deletable {
		p.TotalBytes += c.Bytes
	}
	p.Classes = append(p.Classes, c)
}

func group(rep janitor.Report, executed bool) Preview {
	p := Preview{Classes: []Class{}}
	names := map[janitor.Stage]struct {
		class     string
		deletable bool
	}{
		janitor.StageImages:     {ClassDanglingImages, true},
		janitor.StageBuildCache: {ClassBuildCache, true},
		janitor.StageContainers: {ClassStoppedContainers, true},
		janitor.StageVolumes:    {ClassOrphanedVolumes, false},
	}
	for _, st := range rep.Stages {
		n, ok := names[st.Stage]
		if !ok {
			continue
		}
		c := Class{Name: n.class, Deletable: n.deletable, Items: []janitor.Item{}}
		for _, it := range st.Items {
			if !n.deletable {
				it.Action = janitor.ActionKept
			}
			if executed && n.deletable && it.Action != janitor.ActionRemoved {
				continue
			}
			c.Items = append(c.Items, it)
		}
		if executed && n.deletable {
			c.Bytes = st.ReclaimedBytes
			p.TotalBytes += c.Bytes
			p.Classes = append(p.Classes, c)
			continue
		}
		p.add(c)
	}
	return p
}
