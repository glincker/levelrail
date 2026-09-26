package pathfilter

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		glob, file string
		want       bool
	}{
		{"src/**", "src/a/b/c.go", true},
		{"src/**", "src", true},
		{"src/**", "other/src/a.go", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/main.go", true},
		{"**/*.go", "a/b/main.ts", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
		{"docs/", "docs/a/b.md", true},
		{"docs/**/*.md", "docs/guide.md", true},
		{"docs/**/*.md", "docs/a/b/guide.md", true},
		{"*", ".env", true},
		{"**", ".github/workflows/ci.yml", true},
		{".github/**", ".github/workflows/ci.yml", true},
		{"**/.env*", "app/.env.local", true},
		{"src/?.go", "src/a.go", true},
		{"src/?.go", "src/ab.go", false},
		{"src/[ab].go", "src/b.go", true},
		{"src/[!ab].go", "src/b.go", false},
		{"src/[a-c].go", "src/c.go", true},
		{"*.{ts,tsx}", "x.tsx", true},
		{"*.{ts,tsx}", "x.js", false},
		{`src\**`, `src\a\b.go`, true},
		{"src/**", `src\a\b.go`, true},
		{"./web/**", "web/a.ts", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"", "a", false},
	}
	for _, tt := range tests {
		if got := Match(tt.glob, tt.file); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.glob, tt.file, got, tt.want)
		}
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name  string
		f     Filter
		files []string
		run   bool
		why   string
	}{
		{"no filter", Filter{}, []string{"a"}, true, ""},
		{"unknown files run", Filter{Paths: []string{"src/**"}}, nil, true, ""},
		{"paths hit", Filter{Paths: []string{"src/**"}}, []string{"README.md", "src/a.go"}, true, ""},
		{"paths miss", Filter{Paths: []string{"src/**"}}, []string{"README.md"}, false, "skipped: no changed path matched paths"},
		{"ignore all", Filter{Ignore: []string{"docs/**", "*.md"}}, []string{"docs/a.txt", "README.md"}, false, "skipped: every changed path matched paths_ignore"},
		{"ignore some", Filter{Ignore: []string{"docs/**"}}, []string{"docs/a.txt", "main.go"}, true, ""},
		{"negation via ignore", Filter{Paths: []string{"src/**"}, Ignore: []string{"src/**/*_test.go"}}, []string{"src/a/x_test.go"}, false, "skipped: every changed path matched paths_ignore"},
		{"negation keeps others", Filter{Paths: []string{"src/**"}, Ignore: []string{"src/**/*_test.go"}}, []string{"src/a/x_test.go", "src/b.go"}, true, ""},
		{"dotfile", Filter{Paths: []string{".github/**"}}, []string{".github/ci.yml"}, true, ""},
		{"windows separators", Filter{Paths: []string{"src/**"}}, []string{`src\a.go`}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.Apply(tt.files)
			if got.Run != tt.run || got.Reason != tt.why {
				t.Fatalf("Apply = %+v, want run=%v reason=%q", got, tt.run, tt.why)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		f   Filter
		bad bool
	}{
		{Filter{Paths: []string{"src/**", "*.{a,b}"}}, false},
		{Filter{Paths: []string{" "}}, true},
		{Filter{Ignore: []string{"src/a**b"}}, true},
		{Filter{Paths: []string{"src/[ab"}}, true},
	}
	for _, tt := range tests {
		if err := tt.f.Validate(); (err != nil) != tt.bad {
			t.Errorf("Validate(%+v) err=%v, want bad=%v", tt.f, err, tt.bad)
		}
	}
}
