package deploycontext

import "testing"

func envOf(e Effective, key string) (Entry, bool) {
	for _, x := range e.Env {
		if x.Key == key {
			return x, true
		}
	}
	return Entry{}, false
}

func TestResolveEnvOrder(t *testing.T) {
	base := Inputs{
		Context: DeployPreview,
		Branch:  "release/1",
		AppEnv: map[string]EnvValue{
			"A": {Value: "app"}, "TOKEN": {Secret: true}, "ONLY_APP": {Value: "x"},
		},
		LegacyPreview: map[string]string{"A": "legacy"},
		Contexts: map[string]Config{
			DeployPreview:              {Env: map[string]string{"A": "ctx", "B": "ctx"}},
			BranchContext("release/*"): {Env: map[string]string{"B": "glob", "C": "glob"}},
			BranchContext("release/1"): {Env: map[string]string{"C": "exact"}},
			BranchContext("feature/*"): {Env: map[string]string{"A": "nomatch"}},
		},
		BranchEnv: []BranchOverride{
			{Pattern: "release/*", Key: "B", Value: "ovr-glob"},
			{Pattern: "release/1", Key: "B", Value: "ovr-exact"},
			{Pattern: "release/*", Key: "TOKEN", Secret: true},
		},
	}
	tests := []struct {
		name       string
		mutate     func(*Inputs)
		key        string
		wantValue  string
		wantSource string
		wantSecret bool
	}{
		{"app default survives", nil, "ONLY_APP", "x", SourceApp, false},
		{"context beats legacy preview env", nil, "A", "ctx", SourceContext, false},
		{"branch override beats context and branch context", nil, "B", "ovr-exact", SourceBranchOverride, false},
		{"exact branch context beats glob", nil, "C", "exact", SourceBranchContext, false},
		{"secret override is masked", nil, "TOKEN", MaskedValue, SourceBranchOverride, true},
		{"app secret is masked", func(in *Inputs) { in.Branch = "other" }, "TOKEN", MaskedValue, SourceApp, true},
		{"non matching branch context ignored", func(in *Inputs) { in.Branch = "feature/x" }, "A", "nomatch", SourceBranchContext, false},
		{"production ignores every override", func(in *Inputs) { in.Context = Production }, "A", "app", SourceApp, false},
		{"legacy preview used when no context row", func(in *Inputs) { in.Contexts = nil; in.BranchEnv = nil }, "A", "legacy", SourceLegacyPreview, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			if tt.mutate != nil {
				tt.mutate(&in)
			}
			got, ok := envOf(Resolve(in), tt.key)
			if !ok {
				t.Fatalf("key %q missing", tt.key)
			}
			if got.Value != tt.wantValue || got.Source != tt.wantSource || got.Secret != tt.wantSecret {
				t.Fatalf("got %+v, want value=%q source=%q secret=%v", got, tt.wantValue, tt.wantSource, tt.wantSecret)
			}
		})
	}
}

func TestResolveGlobConflictLastWins(t *testing.T) {
	in := Inputs{
		Context: BranchDeploy, Branch: "release/1",
		BranchEnv: []BranchOverride{
			{Pattern: "release/*", Key: "K", Value: "a"},
			{Pattern: "rel*/1", Key: "K", Value: "b"},
		},
	}
	got, _ := envOf(Resolve(in), "K")
	if got.Value != "a" {
		t.Fatalf("lexicographically last glob should win, got %q", got.Value)
	}
}

func TestResolveBuildSettings(t *testing.T) {
	in := Inputs{
		Context:  BranchDeploy,
		Branch:   "dev",
		AppBuild: Config{BuildCommand: "npm run build", Publish: "dist", BuildArgs: map[string]string{"NODE": "20", "KEEP": "1"}},
		Contexts: map[string]Config{
			BranchDeploy:         {BuildCommand: "npm run build:preview", BuildArgs: map[string]string{"NODE": "22"}},
			BranchContext("dev"): {Publish: "out"},
		},
	}
	eff := Resolve(in)
	if eff.BuildCommand != (Setting{"npm run build:preview", SourceContext}) {
		t.Fatalf("command = %+v", eff.BuildCommand)
	}
	if eff.Publish != (Setting{"out", SourceBranchContext}) {
		t.Fatalf("publish = %+v", eff.Publish)
	}
	if eff.BuildArgs["NODE"] != (Setting{"22", SourceContext}) || eff.BuildArgs["KEEP"] != (Setting{"1", SourceApp}) {
		t.Fatalf("args = %+v", eff.BuildArgs)
	}
}

func TestFor(t *testing.T) {
	tests := []struct {
		pr         int
		branch, pb string
		want       string
	}{
		{5, "feat", "main", DeployPreview},
		{0, "feat", "main", BranchDeploy},
		{0, "main", "main", Production},
		{0, "", "main", Production},
	}
	for _, tt := range tests {
		if got := For(tt.pr, tt.branch, tt.pb); got != tt.want {
			t.Errorf("For(%d,%q,%q) = %q, want %q", tt.pr, tt.branch, tt.pb, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{DeployPreview, false}, {BranchDeploy, false}, {"branch:release/*", false},
		{Production, true}, {"branch:", true}, {"staging", true}, {"branch:[", true},
	}
	for _, tt := range tests {
		if err := Validate(tt.name); (err != nil) != tt.wantErr {
			t.Errorf("Validate(%q) err = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}
