package webhook

import (
	"errors"
	"testing"
)

func TestParseBitbucketServerPushEvent(t *testing.T) {
	const nullHash = bitbucketServerNullHash
	repo := `"repository":{"slug":"app","project":{"key":"PROJ"},"links":{"self":[{"href":"https://bb/projects/PROJ/repos/app/browse"}]}}`
	cases := []struct {
		name    string
		body    string
		want    PushEvent
		wantErr error
	}{
		{
			name: "branch update",
			body: `{` + repo + `,"changes":[{"ref":{"id":"refs/heads/main","type":"BRANCH"},"fromHash":"aaa","toHash":"bbb","type":"UPDATE"}]}`,
			want: PushEvent{Ref: "refs/heads/main", Before: "aaa", After: "bbb", RepoNames: []string{"PROJ/app"}, RepoURLs: []string{"https://bb/projects/PROJ/repos/app/browse"}},
		},
		{
			name: "new branch has no before",
			body: `{` + repo + `,"changes":[{"ref":{"id":"refs/heads/dev","type":"BRANCH"},"fromHash":"` + nullHash + `","toHash":"ccc","type":"ADD"}]}`,
			want: PushEvent{Ref: "refs/heads/dev", After: "ccc", RepoNames: []string{"PROJ/app"}, RepoURLs: []string{"https://bb/projects/PROJ/repos/app/browse"}},
		},
		{
			name: "skips tag and delete, takes next branch",
			body: `{` + repo + `,"changes":[{"ref":{"id":"refs/tags/v1","type":"TAG"},"toHash":"t","type":"ADD"},{"ref":{"id":"refs/heads/old","type":"BRANCH"},"toHash":"` + nullHash + `","type":"DELETE"},{"ref":{"id":"refs/heads/x","type":"BRANCH"},"fromHash":"a","toHash":"d","type":"UPDATE"}]}`,
			want: PushEvent{Ref: "refs/heads/x", Before: "a", After: "d", RepoNames: []string{"PROJ/app"}, RepoURLs: []string{"https://bb/projects/PROJ/repos/app/browse"}},
		},
		{
			name:    "only delete",
			body:    `{` + repo + `,"changes":[{"ref":{"id":"refs/heads/old"},"toHash":"` + nullHash + `","type":"DELETE"}]}`,
			wantErr: ErrPushEventFieldsMissing,
		},
		{name: "malformed", body: `{`, wantErr: errAny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePushEventForProvider([]byte(tc.body), "repo:refs_changed")
			switch {
			case errors.Is(tc.wantErr, errAny):
				if err == nil {
					t.Fatal("want error")
				}
				return
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			if got.Ref != tc.want.Ref || got.Before != tc.want.Before || got.After != tc.want.After ||
				len(got.RepoNames) != 1 || got.RepoNames[0] != tc.want.RepoNames[0] || got.RepoURLs[0] != tc.want.RepoURLs[0] {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

var errAny = errors.New("any error")
