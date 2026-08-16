package docker

import (
	"strings"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

// anonID64 is a well-formed Docker anonymous-volume ID shape: exactly
// 64 lowercase hex characters. Built from a repeated pattern rather than
// hand-typed so its length is trivially verifiable.
var anonID64 = strings.Repeat("0123456789abcdef", 4)

func TestIsAnonymousVolumeName(t *testing.T) {
	if len(anonID64) != 64 {
		t.Fatalf("test fixture anonID64 has length %d, want 64", len(anonID64))
	}
	tests := []struct {
		name string
		vol  string
		want bool
	}{
		{
			name: "64 char lowercase hex is anonymous",
			vol:  anonID64,
			want: true,
		},
		{
			name: "named database volume is not anonymous",
			vol:  "db-main-data",
			want: false,
		},
		{
			name: "empty string is not anonymous",
			vol:  "",
			want: false,
		},
		{
			name: "too short to be a real anonymous id",
			vol:  anonID64[:6],
			want: false,
		},
		{
			name: "uppercase hex does not match Docker's own lowercase id format",
			vol:  strings.ToUpper(anonID64),
			want: false,
		},
		{
			name: "64 chars but contains a non-hex character",
			vol:  "g" + anonID64[1:],
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnonymousVolumeName(tt.vol); got != tt.want {
				t.Errorf("isAnonymousVolumeName(%q) = %v, want %v", tt.vol, got, tt.want)
			}
		})
	}
}

func containerSummary(id, name string) container.Summary {
	return container.Summary{ID: id, Names: []string{"/" + name}}
}

func TestStoppedNotKept(t *testing.T) {
	all := []container.Summary{
		containerSummary("c1", "web-abc12345"),
		containerSummary("c2", "web-abc12345-r1"),
		containerSummary("c3", "db-main"),
		containerSummary("c4", "some-unrelated-leftover"),
	}

	tests := []struct {
		name string
		keep []string
		want []string // expected surviving-as-candidate IDs
	}{
		{
			name: "nothing kept, every stopped container is a candidate",
			keep: nil,
			want: []string{"c1", "c2", "c3", "c4"},
		},
		{
			name: "desired application and database containers excluded",
			keep: []string{"web-abc12345", "web-abc12345-r1", "db-main"},
			want: []string{"c4"},
		},
		{
			name: "keep entries that match nothing on this daemon are harmless no-ops",
			keep: []string{"nonexistent-service-deadbeef"},
			want: []string{"c1", "c2", "c3", "c4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stoppedNotKept(all, tt.keep)
			gotIDs := make([]string, 0, len(got))
			for _, cs := range got {
				gotIDs = append(gotIDs, cs.ID)
			}
			if !equalStrings(gotIDs, tt.want) {
				t.Errorf("stoppedNotKept() ids = %v, want %v", gotIDs, tt.want)
			}
		})
	}
}

func TestContainerSummaryName(t *testing.T) {
	tests := []struct {
		name string
		cs   container.Summary
		want string
	}{
		{name: "strips leading slash", cs: containerSummary("c1", "web"), want: "web"},
		{name: "no names at all", cs: container.Summary{ID: "c1"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containerSummaryName(tt.cs); got != tt.want {
				t.Errorf("containerSummaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAggregateDiskUsage(t *testing.T) {
	used := int64(0)
	unusedTagged := int64(500)
	dangling := int64(300)

	du := dockertypes.DiskUsage{
		Images: []*image.Summary{
			{ID: "used", Size: 1000, Containers: 1},
			{ID: "unused-tagged", Size: unusedTagged, Containers: 0, RepoTags: []string{"app:old"}},
			{ID: "dangling", Size: dangling, Containers: 0, RepoTags: nil},
		},
		Containers: []*container.Summary{
			{ID: "running", SizeRw: 10, State: "running"},
			{ID: "stopped", SizeRw: 20, State: "exited"},
		},
		Volumes: []*volume.Volume{
			{Name: "db-main-data", UsageData: &volume.UsageData{RefCount: 1, Size: 100}},
			{Name: "anon-unused", UsageData: &volume.UsageData{RefCount: 0, Size: 50}},
			{Name: "no-usage-data-reported"}, // UsageData nil: driver doesn't report size
		},
		BuildCache: []*build.CacheRecord{
			{ID: "active", Size: 7, InUse: true},
			{ID: "idle", Size: 3, InUse: false},
		},
	}
	_ = used

	got := aggregateDiskUsage(du)

	if got.ImagesTotalBytes != 1000+unusedTagged+dangling {
		t.Errorf("ImagesTotalBytes = %d, want %d", got.ImagesTotalBytes, 1000+unusedTagged+dangling)
	}
	if got.ImagesReclaimableBytes != unusedTagged+dangling {
		t.Errorf("ImagesReclaimableBytes = %d, want %d (both unused images, tagged or not: matches `docker system df`, wider than what PruneDanglingImages actually removes)", got.ImagesReclaimableBytes, unusedTagged+dangling)
	}

	if got.ContainersTotalBytes != 30 {
		t.Errorf("ContainersTotalBytes = %d, want 30", got.ContainersTotalBytes)
	}
	if got.ContainersReclaimableBytes != 20 {
		t.Errorf("ContainersReclaimableBytes = %d, want 20 (only the stopped one)", got.ContainersReclaimableBytes)
	}

	if got.VolumesTotalBytes != 150 {
		t.Errorf("VolumesTotalBytes = %d, want 150 (100+50, the nil-UsageData volume excluded as unknown)", got.VolumesTotalBytes)
	}
	if got.VolumesReclaimableBytes != 50 {
		t.Errorf("VolumesReclaimableBytes = %d, want 50 (only the RefCount==0 volume)", got.VolumesReclaimableBytes)
	}

	if got.BuildCacheTotalBytes != 10 {
		t.Errorf("BuildCacheTotalBytes = %d, want 10", got.BuildCacheTotalBytes)
	}
	if got.BuildCacheReclaimableBytes != 3 {
		t.Errorf("BuildCacheReclaimableBytes = %d, want 3 (only the not-InUse record)", got.BuildCacheReclaimableBytes)
	}
}

func TestAggregateDiskUsage_Empty(t *testing.T) {
	got := aggregateDiskUsage(dockertypes.DiskUsage{})
	want := DiskUsage{}
	if got != want {
		t.Errorf("aggregateDiskUsage(empty) = %+v, want zero value", got)
	}
}

func TestVolumeSize(t *testing.T) {
	tests := []struct {
		name string
		vol  *volume.Volume
		want int64
	}{
		{name: "nil volume", vol: nil, want: -1},
		{name: "nil usage data", vol: &volume.Volume{Name: "v"}, want: -1},
		{name: "usage data present", vol: &volume.Volume{Name: "v", UsageData: &volume.UsageData{Size: 42}}, want: 42},
		{name: "driver reports unavailable as negative", vol: &volume.Volume{Name: "v", UsageData: &volume.UsageData{Size: -1}}, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := volumeSize(tt.vol); got != tt.want {
				t.Errorf("volumeSize() = %d, want %d", got, tt.want)
			}
		})
	}
}
