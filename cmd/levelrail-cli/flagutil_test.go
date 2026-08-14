package main

import (
	"flag"
	"reflect"
	"testing"
)

func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("token", "", "")
	fs.String("api-url", "", "")
	fs.Bool("json", false, "")
	return fs
}

func TestReorderArgsFlagsFirst(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "already flags first",
			args: []string{"--token", "t", "--json", "web"},
			want: []string{"--token", "t", "--json", "web"},
		},
		{
			name: "positional first, value flag after",
			args: []string{"web", "--token", "t"},
			want: []string{"--token", "t", "web"},
		},
		{
			name: "positional first, bool flag after",
			args: []string{"web", "--json"},
			want: []string{"--json", "web"},
		},
		{
			name: "mixed order, several flags",
			args: []string{"web", "--json", "--api-url", "http://x", "--token", "t"},
			want: []string{"--json", "--api-url", "http://x", "--token", "t", "web"},
		},
		{
			name: "equals form carries its own value",
			args: []string{"web", "--token=t"},
			want: []string{"--token=t", "web"},
		},
		{
			name: "double dash stops flag interpretation",
			args: []string{"--token", "t", "--", "--json"},
			want: []string{"--token", "t", "--json"},
		},
		{
			name: "unknown flag is left alone, not treated as taking a value",
			args: []string{"web", "--bogus", "extra"},
			want: []string{"--bogus", "web", "extra"},
		},
		{
			name: "no positional args at all",
			args: []string{"--token", "t", "--json"},
			want: []string{"--token", "t", "--json"},
		},
		{
			name: "no flags at all",
			args: []string{"web"},
			want: []string{"web"},
		},
		{
			name: "empty",
			args: nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgsFlagsFirst(testFlagSet(), tt.args)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorderArgsFlagsFirst(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// TestReorderArgsFlagsFirst_ParsesCorrectly proves the fix actually
// works end to end against a real flag.FlagSet.Parse call, not just
// that the reordering looks right: the bug this exists to fix
// (cmd/levelrail-cli/apps_get.go, discovered live against a real
// control plane) was "apps get web --json" silently leaving --json
// unset because stdlib flag stops at the first positional argument.
func TestReorderArgsFlagsFirst_ParsesCorrectly(t *testing.T) {
	fs := testFlagSet()
	args := reorderArgsFlagsFirst(fs, []string{"web", "--json", "--token", "t"})
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse(%v) error = %v", args, err)
	}
	if got := fs.Lookup("json").Value.String(); got != "true" {
		t.Errorf("json flag = %q, want \"true\"", got)
	}
	if got := fs.Lookup("token").Value.String(); got != "t" {
		t.Errorf("token flag = %q, want \"t\"", got)
	}
	if rest := fs.Args(); len(rest) != 1 || rest[0] != "web" {
		t.Errorf("positional args = %v, want [web]", rest)
	}
}
