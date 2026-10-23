package repolist

import (
	"fmt"
	"testing"
)

func sample() []Repo {
	return []Repo{
		{FullName: "acme/api", Owner: "acme"},
		{FullName: "acme/web", Owner: "acme", Archived: true},
		{FullName: "acme/fork-of-x", Owner: "acme", Fork: true},
		{FullName: "someone/api", Owner: "someone"},
		{FullName: "Zed/tool", Owner: "Zed"},
	}
}

func names(r []Repo) []string {
	out := make([]string, 0, len(r))
	for _, x := range r {
		out = append(out, x.FullName)
	}
	return out
}

func TestApplyFilters(t *testing.T) {
	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"all, ordered by name", Query{}, []string{"acme/api", "acme/fork-of-x", "acme/web", "someone/api", "Zed/tool"}},
		{"search narrows", Query{Search: "api"}, []string{"acme/api", "someone/api"}},
		{"search terms all match", Query{Search: "acme web"}, []string{"acme/web"}},
		{"owner filter is case insensitive", Query{Owner: "zed"}, []string{"Zed/tool"}},
		{"archived excluded", Query{Archived: Exclude}, []string{"acme/api", "acme/fork-of-x", "someone/api", "Zed/tool"}},
		{"archived only", Query{Archived: Only}, []string{"acme/web"}},
		{"forks excluded", Query{Forks: Exclude}, []string{"acme/api", "acme/web", "someone/api", "Zed/tool"}},
		{"forks only", Query{Forks: Only}, []string{"acme/fork-of-x"}},
		{"same name under two owners stays distinct", Query{Search: "/api"}, []string{"acme/api", "someone/api"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(Apply(sample(), tt.q).Items)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyPagingPastAThousand(t *testing.T) {
	repos := make([]Repo, 2500)
	for i := range repos {
		repos[i] = Repo{FullName: fmt.Sprintf("o/r%05d", i), Owner: "o"}
	}
	p := Apply(repos, Query{Limit: 1000})
	if len(p.Items) != 500 || p.Total != 2500 || p.NextOffset != 500 {
		t.Fatalf("limit is capped at MaxLimit: items=%d total=%d next=%d", len(p.Items), p.Total, p.NextOffset)
	}
	last := Apply(repos, Query{Limit: 500, Offset: 2400})
	if len(last.Items) != 100 || last.NextOffset != -1 || last.Items[99].FullName != "o/r02499" {
		t.Fatalf("last page: items=%d next=%d", len(last.Items), last.NextOffset)
	}
	beyond := Apply(repos, Query{Offset: 9999})
	if len(beyond.Items) != 0 || beyond.NextOffset != -1 {
		t.Fatalf("offset past the end: %+v", beyond)
	}
}

func TestApplyOwnerFacetsIgnoreOwnerAndSearchButHonorToggles(t *testing.T) {
	p := Apply(sample(), Query{Search: "tool", Archived: Exclude})
	if len(p.Owners) != 3 || p.Owners[0].Login != "acme" || p.Owners[0].Count != 2 {
		t.Fatalf("owners = %+v, want acme(2, archived excluded), someone, Zed", p.Owners)
	}
}
