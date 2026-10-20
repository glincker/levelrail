package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const edgeTagPrefix = "main-"

// runningInContainer is a variable so tests can pick either install shape.
var runningInContainer = func() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// edgeGuidance tells an operator what the edge channel means for this
// install: edge is a moving container image tag, never a signed binary.
type edgeGuidance struct {
	Commit          string `json:"commit,omitempty"`
	CommitURL       string `json:"commit_url,omitempty"`
	Image           string `json:"image"`
	RunsInContainer bool   `json:"runs_in_container"`
	// Installable is true only where the image can be pulled and recreated.
	Installable     bool   `json:"installable"`
	PullCommand     string `json:"pull_command,omitempty"`
	RecreateCommand string `json:"recreate_command,omitempty"`
	ComposeSnippet  string `json:"compose_snippet,omitempty"`
	// SwitchTo lists the channels a binary install can move to in one click.
	SwitchTo []string `json:"switch_to,omitempty"`
}

func buildEdgeGuidance(release *upgrade.Release) *edgeGuidance {
	if !release.IsEdge() {
		return nil
	}
	image := release.Image
	if image == "" {
		image = upgrade.EdgeImage(githubRepo)
	}
	g := &edgeGuidance{Commit: release.Commit, CommitURL: release.URL, Image: image, RunsInContainer: runningInContainer()}
	if !g.RunsInContainer {
		g.SwitchTo = []string{upgrade.ChannelBeta, upgrade.ChannelStable}
		return g
	}
	svc := "app"
	if exe, err := os.Executable(); err == nil {
		svc = filepath.Base(exe)
	}
	g.Installable = true
	g.PullCommand = "docker pull " + image
	g.RecreateCommand = "docker compose pull " + svc + " && docker compose up -d " + svc
	g.ComposeSnippet = "services:\n  " + svc + ":\n    image: " + image + "\n"
	return g
}

// isEdgeTag reports whether tag names an edge build ("main-<sha>").
func isEdgeTag(tag string) bool {
	return strings.HasPrefix(strings.TrimSpace(tag), edgeTagPrefix)
}

func releaseKind(release *upgrade.Release) string {
	if release.IsEdge() {
		return upgrade.KindEdgeImage
	}
	return upgrade.KindRelease
}
