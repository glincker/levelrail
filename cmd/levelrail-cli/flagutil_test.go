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

func TestStringMapFlag_Set(t *testing.T) {
	tests := []struct {
		name    string
		sets    []string
		want    map[string]string
		wantErr bool
	}{
		{name: "single pair", sets: []string{"FOO=bar"}, want: map[string]string{"FOO": "bar"}},
		{name: "multiple pairs accumulate", sets: []string{"FOO=bar", "BAZ=qux"}, want: map[string]string{"FOO": "bar", "BAZ": "qux"}},
		{name: "value contains equals", sets: []string{"URL=http://x?a=b"}, want: map[string]string{"URL": "http://x?a=b"}},
		{name: "empty value is valid", sets: []string{"FOO="}, want: map[string]string{"FOO": ""}},
		{name: "no equals sign rejected", sets: []string{"FOO"}, wantErr: true},
		{name: "empty key rejected", sets: []string{"=bar"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := make(stringMapFlag)
			var err error
			for _, s := range tt.sets {
				if err = m.Set(s); err != nil {
					break
				}
			}
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Set() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Set() error = %v", err)
			}
			if !reflect.DeepEqual(map[string]string(m), tt.want) {
				t.Errorf("map = %+v, want %+v", map[string]string(m), tt.want)
			}
		})
	}
}

func TestStringMapFlag_Var(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	m := make(stringMapFlag)
	fs.Var(m, "build-arg", "")
	if err := fs.Parse([]string{"--build-arg", "FOO=bar", "--build-arg", "BAZ=qux"}); err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]string{"FOO": "bar", "BAZ": "qux"}
	if !reflect.DeepEqual(map[string]string(m), want) {
		t.Errorf("map = %+v, want %+v", map[string]string(m), want)
	}
}
