package webhook

import (
	"errors"
	"testing"
)

func TestParseBitbucketPushEvent(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    PushEvent
		wantErr error
	}{
		{
			name: "ordinary push",
			body: `{"push":{"changes":[{"new":{"type":"branch","name":"main","target":{"hash":"abc123"}},"old":{"type":"branch","name":"main","target":{"hash":"def456"}}}]}}`,
			want: PushEvent{Ref: "refs/heads/main", After: "abc123"},
		},
		{
			name: "new branch push (no old)",
			body: `{"push":{"changes":[{"new":{"type":"branch","name":"feature/x","target":{"hash":"c0ffee"}}}]}}`,
			want: PushEvent{Ref: "refs/heads/feature/x", After: "c0ffee"},
		},
		{
			name:    "branch delete (new is null)",
			body:    `{"push":{"changes":[{"new":null,"old":{"type":"branch","name":"main","target":{"hash":"def456"}}}]}}`,
			wantErr: ErrPushEventFieldsMissing,
		},
		{
			name:    "no changes",
			body:    `{"push":{"changes":[]}}`,
			wantErr: ErrPushEventFieldsMissing,
		},
		{
			name:    "malformed json",
			body:    `{not json`,
			wantErr: nil, // any non-nil error is fine, checked separately below
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBitbucketPushEvent([]byte(tt.body))
			if tt.name == "malformed json" {
				if err == nil {
					t.Fatal("ParseBitbucketPushEvent() error = nil, want a decode error")
				}
				return
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseBitbucketPushEvent() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBitbucketPushEvent() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Errorf("ParseBitbucketPushEvent() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParsePushEventForProvider(t *testing.T) {
	githubBody := `{"ref":"refs/heads/main","after":"gh123"}`
	bitbucketBody := `{"push":{"changes":[{"new":{"name":"main","target":{"hash":"bb456"}}}]}}`

	t.Run("github event key falls through to ParsePushEvent", func(t *testing.T) {
		got, err := ParsePushEventForProvider([]byte(githubBody), "")
		if err != nil {
			t.Fatalf("ParsePushEventForProvider() error = %v", err)
		}
		if got.After != "gh123" {
			t.Errorf("got = %+v, want the GitHub-shaped payload's after", got)
		}
	})

	t.Run("gitlab shares github's ref/after shape, no event key match", func(t *testing.T) {
		got, err := ParsePushEventForProvider([]byte(githubBody), "Push Hook")
		if err != nil {
			t.Fatalf("ParsePushEventForProvider() error = %v", err)
		}
		if got.After != "gh123" {
			t.Errorf("got = %+v, want the ref/after-shaped payload's after", got)
		}
	})

	t.Run("bitbucket event key routes to the bitbucket parser", func(t *testing.T) {
		got, err := ParsePushEventForProvider([]byte(bitbucketBody), "repo:push")
		if err != nil {
			t.Fatalf("ParsePushEventForProvider() error = %v", err)
		}
		if got.Ref != "refs/heads/main" || got.After != "bb456" {
			t.Errorf("got = %+v, want the normalized bitbucket payload", got)
		}
	})
}
