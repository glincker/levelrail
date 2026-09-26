package webhook

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func manyCommits(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf(`{"id":"c%d","added":["f%d.txt"],"modified":[],"removed":[]}`, i, i))
	}
	return strings.Join(parts, ",")
}

func TestParsePushEventChangedFiles(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "github union across commits",
			body: `{"ref":"refs/heads/main","after":"a2","before":"a0","head_commit":{"id":"a2"},"commits":[
				{"id":"a1","added":["src/new.go"],"modified":["README.md"],"removed":[]},
				{"id":"a2","added":[],"modified":["README.md","src/x.go"],"removed":["old.txt"]}]}`,
			want: []string{"src/new.go", "README.md", "src/x.go", "old.txt"},
		},
		{
			name: "gitlab push hook shape",
			body: `{"ref":"refs/heads/main","after":"b1","before":"b0","commits":[{"id":"b1","added":[],"modified":["app/main.rb"],"removed":[]}]}`,
			want: []string{"app/main.rb"},
		},
		{
			name: "gitea push shape",
			body: `{"ref":"refs/heads/main","after":"c1","before":"c0","commits":[{"id":"c1","added":["a"],"modified":[],"removed":[]}]}`,
			want: []string{"a"},
		},
		{
			name: "no commits (tag push) is unknown",
			body: `{"ref":"refs/tags/v1","after":"d1","commits":[]}`,
			want: nil,
		},
		{
			name: "truncated commit list is unknown",
			body: `{"ref":"refs/heads/main","after":"e1","commits":[` + manyCommits(maxPayloadCommits) + `]}`,
			want: nil,
		},
		{
			name: "just under the truncation limit is trusted",
			body: `{"ref":"refs/heads/main","after":"e1","commits":[` + manyCommits(maxPayloadCommits-1) + `]}`,
			want: func() []string {
				var out []string
				for i := 0; i < maxPayloadCommits-1; i++ {
					out = append(out, fmt.Sprintf("f%d.txt", i))
				}
				return out
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := ParsePushEvent([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(ev.Changed, tt.want) {
				t.Fatalf("Changed = %v, want %v", ev.Changed, tt.want)
			}
		})
	}
}

func TestBitbucketPushHasNoFileList(t *testing.T) {
	ev, err := ParseBitbucketPushEvent([]byte(`{"push":{"changes":[{"new":{"name":"main","target":{"hash":"n1"}},"old":{"target":{"hash":"o1"}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Changed != nil || ev.Before != "o1" || ev.After != "n1" {
		t.Fatalf("event = %+v, want no Changed and before/after set for the compare fallback", ev)
	}
}

func TestParseMergeGroupEvent(t *testing.T) {
	body := `{"action":"checks_requested","merge_group":{"head_sha":"h1","head_ref":"refs/heads/gh-readonly-queue/main/pr-3-x","base_ref":"refs/heads/main","base_sha":"b1"}}`
	ev, err := ParseMergeGroupEvent([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.ChecksRequested() || ev.HeadSHA != "h1" || ev.BaseBranch() != "main" || ev.HeadRef != "refs/heads/gh-readonly-queue/main/pr-3-x" {
		t.Fatalf("event = %+v", ev)
	}
	destroyed, err := ParseMergeGroupEvent([]byte(strings.Replace(body, "checks_requested", "destroyed", 1)))
	if err != nil || destroyed.ChecksRequested() {
		t.Fatalf("destroyed event = %+v, err %v", destroyed, err)
	}
	if _, err := ParseMergeGroupEvent([]byte(`{"action":"checks_requested","merge_group":{}}`)); err == nil {
		t.Fatal("payload without head_sha parsed")
	}
	if _, err := ParseMergeGroupEvent([]byte(`{bad`)); err == nil {
		t.Fatal("malformed payload parsed")
	}
}

func TestIsMergeGroupEvent(t *testing.T) {
	h := http.Header{}
	h.Set("X-GitHub-Event", "merge_group")
	if !IsMergeGroupEvent(h) {
		t.Fatal("merge_group header not recognized")
	}
	h.Set("X-GitHub-Event", "push")
	if IsMergeGroupEvent(h) {
		t.Fatal("push treated as merge_group")
	}
}
