package webhook

import (
	"testing"
	"time"
)

func TestParsePushEventOrdering(t *testing.T) {
	want := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name, body, key string
		wantBefore      string
		wantAt          time.Time
	}{
		{name: "github head_commit", body: `{"ref":"refs/heads/main","before":"aaa","after":"bbb","head_commit":{"id":"bbb","timestamp":"2026-09-25T12:00:00+02:00"}}`, wantBefore: "aaa", wantAt: want},
		{name: "gitlab commits list", body: `{"ref":"refs/heads/main","before":"aaa","after":"bbb","commits":[{"id":"x","timestamp":"2020-01-01T00:00:00Z"},{"id":"bbb","timestamp":"2026-09-25T10:00:00Z"}]}`, wantBefore: "aaa", wantAt: want},
		{name: "no timestamp", body: `{"ref":"refs/heads/main","after":"bbb"}`},
		{name: "bitbucket", key: "repo:push", body: `{"push":{"changes":[{"old":{"name":"main","target":{"hash":"aaa"}},"new":{"name":"main","target":{"hash":"bbb","date":"2026-09-25T10:00:00+00:00"}}}]}}`, wantBefore: "aaa", wantAt: want},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, err := ParsePushEventForProvider([]byte(tt.body), tt.key)
			if err != nil {
				t.Fatal(err)
			}
			if ev.After != "bbb" || ev.Before != tt.wantBefore || !ev.HeadCommitAt.Equal(tt.wantAt) {
				t.Fatalf("got %+v", ev)
			}
		})
	}
}
