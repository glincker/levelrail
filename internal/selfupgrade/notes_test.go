package selfupgrade

import (
	"reflect"
	"testing"
)

func TestParseBreaking(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Breaking
	}{
		{name: "empty", body: ""},
		{name: "no breaking section", body: "## Features\n- a thing\n"},
		{
			name: "metadata comment",
			body: "Intro\n<!-- levelrail-upgrade: {\"breaking\":[{\"id\":\"Ingress Ports\",\"summary\":\"Ports move\",\"ack\":true},{\"id\":\"cli-flag\",\"summary\":\"flag renamed\",\"ack\":false}]} -->",
			want: []Breaking{
				{ID: "ingress-ports", Version: "v1.0.0", Summary: "Ports move", RequiresAck: true, Source: SourceMetadata},
				{ID: "cli-flag", Version: "v1.0.0", Summary: "flag renamed", RequiresAck: false, Source: SourceMetadata},
			},
		},
		{
			name: "release-please heading",
			body: "### Features\n* x\n\n### ⚠ BREAKING CHANGES\n\n* drop the old route\n* rename the env var\n\n### Bug Fixes\n* y\n",
			want: []Breaking{
				{ID: stableID("v1.0.0", "drop the old route"), Version: "v1.0.0", Summary: "drop the old route", RequiresAck: true, Source: SourceHeading},
				{ID: stableID("v1.0.0", "rename the env var"), Version: "v1.0.0", Summary: "rename the env var", RequiresAck: true, Source: SourceHeading},
			},
		},
		{
			name: "broken metadata falls back to heading",
			body: "<!-- levelrail-upgrade: {nope} -->\n## Breaking changes\n- one\n",
			want: []Breaking{{ID: stableID("v1.0.0", "one"), Version: "v1.0.0", Summary: "one", RequiresAck: true, Source: SourceHeading}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBreaking("v1.0.0", tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestBreakingBetweenAndAcks(t *testing.T) {
	rels := []ReleaseNotes{
		{Tag: "v0.6.0", Body: "## Breaking changes\n- six\n"},
		{Tag: "v0.4.0", Body: "## Breaking changes\n- current, not newer\n"},
		{Tag: "v0.5.0", Body: "## Breaking changes\n- five\n"},
		{Tag: "v0.7.0", Body: "## Breaking changes\n- beyond target\n"},
		{Tag: "weird", Body: "## Breaking changes\n- unordered\n"},
	}
	got := BreakingBetween(rels, "v0.4.0", "v0.6.0")
	if len(got) != 2 || got[0].Version != "v0.5.0" || got[1].Version != "v0.6.0" {
		t.Fatalf("got %+v", got)
	}
	missing := MissingAcks(got, []string{got[0].ID})
	if len(missing) != 1 || missing[0].ID != got[1].ID {
		t.Fatalf("missing = %+v", missing)
	}
	if len(MissingAcks(got, []string{got[0].ID, got[1].ID})) != 0 {
		t.Fatal("all acknowledged but still missing")
	}
}
