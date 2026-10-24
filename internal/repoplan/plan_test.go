package repoplan

import (
	"strings"
	"testing"
)

func nameFor(dir string) string { return "mono-" + dir[strings.LastIndex(dir, "/")+1:] }

func codes(ws []Warning) string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return strings.Join(out, ",")
}

func TestBuild(t *testing.T) {
	pkgs := []Package{
		{PackageName: "web", Dir: "apps/web", Dockerfile: "apps/web/Dockerfile"},
		{PackageName: "api", Dir: "apps/api"},
		{PackageName: "web2", Dir: "services/web"},
		{PackageName: "docs", Dir: "apps/docs"},
	}
	detect := map[string]Detection{
		"apps/web":     {Port: 3000, PortSource: "EXPOSE", Kind: KindDockerfile},
		"apps/api":     {Port: 3000, PortSource: "Express convention", Kind: KindBuildpack},
		"services/web": {Port: 8080, Kind: KindDockerfile},
		"apps/docs":    {Kind: KindStatic},
	}
	tests := []struct {
		name      string
		in        Input
		wantNames []string
		wantWarn  map[string]string
		planWarn  string
		wantFlag  int
	}{
		{
			name:      "collisions and duplicate ports",
			in:        Input{Packages: pkgs, Found: 4, Cap: 60, Detect: detect, Existing: map[string]bool{"mono-api": true}},
			wantNames: []string{"mono-api-2", "mono-docs", "mono-web", "mono-web-2"},
			wantWarn: map[string]string{
				"apps/api":     "no_dockerfile,name_taken,duplicate_port",
				"apps/docs":    "no_dockerfile",
				"apps/web":     "duplicate_port",
				"services/web": "duplicate_name",
			},
			wantFlag: 4,
		},
		{
			name:      "over the cap",
			in:        Input{Packages: pkgs[:1], Found: 90, Cap: 1, Detect: detect},
			wantNames: []string{"mono-web"},
			planWarn:  "package_cap",
			wantFlag:  0,
		},
		{
			name:     "no workspace",
			in:       Input{Cap: 60},
			planWarn: "no_workspace",
		},
		{
			name:      "base directory filters",
			in:        Input{Packages: pkgs, Found: 4, Cap: 60, Detect: detect, BaseDirectory: "services/"},
			wantNames: []string{"mono-web"},
			wantWarn:  map[string]string{"services/web": ""},
		},
		{
			name:      "no detection means no port warning flood",
			in:        Input{Packages: pkgs[1:2], Found: 1, Cap: 60, Detect: map[string]Detection{"apps/api": {Failed: true}}},
			wantNames: []string{"mono-api"},
			wantWarn:  map[string]string{"apps/api": "detect_failed,no_dockerfile"},
		},
		{
			name:      "missing port warns",
			in:        Input{Packages: pkgs[1:2], Found: 1, Cap: 60, Detect: map[string]Detection{"apps/api": {Kind: KindBuildpack}}},
			wantNames: []string{"mono-api"},
			wantWarn:  map[string]string{"apps/api": "no_dockerfile,no_port"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.NameFor = nameFor
			tc.in.MaxNameLen = 40
			p := Build(tc.in)
			var got []string
			for _, r := range p.Packages {
				got = append(got, r.Name)
				if want, ok := tc.wantWarn[r.Directory]; ok && codes(r.Warnings) != want {
					t.Errorf("%s warnings = %q, want %q", r.Directory, codes(r.Warnings), want)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.wantNames, ",") {
				t.Fatalf("names = %v, want %v", got, tc.wantNames)
			}
			if codes(p.Warnings) != tc.planWarn {
				t.Errorf("plan warnings = %q, want %q", codes(p.Warnings), tc.planWarn)
			}
			if tc.wantFlag != 0 && p.Totals.Flagged != tc.wantFlag {
				t.Errorf("flagged = %d, want %d", p.Totals.Flagged, tc.wantFlag)
			}
			if p.Totals.Packages != len(p.Packages) {
				t.Errorf("totals.packages = %d", p.Totals.Packages)
			}
		})
	}
}

func TestBuildLongNameSuffixStaysBounded(t *testing.T) {
	long := strings.Repeat("a", 40)
	p := Build(Input{
		Packages: []Package{{Dir: "x/one"}}, Found: 1, Cap: 60, MaxNameLen: 40,
		Existing: map[string]bool{long: true}, NameFor: func(string) string { return long },
	})
	if got := p.Packages[0].Name; len(got) > 40 || !strings.HasSuffix(got, "-2") {
		t.Fatalf("name = %q", got)
	}
}
