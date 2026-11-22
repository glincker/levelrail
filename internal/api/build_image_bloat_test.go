package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestImageBloatThresholdsAndGrownLayer(t *testing.T) {
	p := bloatPolicy{Percent: 30, MinBytes: 50 * bytesPerMiB}
	big := imageChange{Delta: 200 * bytesPerMiB, PrevBytes: 400 * bytesPerMiB, PrevTag: "app:1"}
	if b := p.check(big, "app:2"); b == nil || b.GrowthPercent != 50 {
		t.Fatalf("200 MiB on 400 MiB must warn at 50 percent, got %+v", b)
	}
	if p.check(imageChange{Delta: 10 * bytesPerMiB, PrevBytes: 20 * bytesPerMiB}, "t") != nil {
		t.Fatal("growth under the minimum must stay quiet")
	}
	if p.check(imageChange{Delta: 60 * bytesPerMiB, PrevBytes: 1000 * bytesPerMiB}, "t") != nil {
		t.Fatal("growth under the percent must stay quiet")
	}
	if (bloatPolicy{}).check(big, "t") != nil {
		t.Fatal("percent 0 turns the warning off")
	}

	prev := []docker.ImageLayer{{CreatedBy: "RUN npm ci", SizeBytes: 100}, {CreatedBy: "COPY . .", SizeBytes: 10}}
	cur := []docker.ImageLayer{{CreatedBy: "RUN npm ci", SizeBytes: 100}, {CreatedBy: "COPY . .", SizeBytes: 15}, {CreatedBy: "/bin/sh -c apt-get install ffmpeg", SizeBytes: 300}}
	name, grown := grownLayer(prev, cur)
	if name != "apt-get install ffmpeg" || grown != 300 {
		t.Fatalf("grown layer = %q %d", name, grown)
	}
}
