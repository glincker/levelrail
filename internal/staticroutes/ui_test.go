package staticroutes

import "testing"

func TestCompileUIValidation(t *testing.T) {
	cases := []struct {
		name      string
		redirects []UIRedirect
		headers   []UIHeader
		wantProbs int
	}{
		{"valid", []UIRedirect{{From: "/a", To: "/b"}}, []UIHeader{{Path: "/*", Set: map[string]string{"X-A": "b"}}}, 0},
		{"bad status", []UIRedirect{{From: "/a", To: "/b", Status: 599}}, nil, 1},
		{"empty to", []UIRedirect{{From: "/a"}}, nil, 1},
		{"reserved header", nil, []UIHeader{{Path: "/*", Set: map[string]string{"Host": "x"}}}, 1},
		{"both bad", []UIRedirect{{From: "/a"}}, []UIHeader{{Path: "nope"}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, probs := CompileUI(tc.redirects, tc.headers, DefaultLimits())
			if len(probs) != tc.wantProbs {
				t.Fatalf("problems = %+v, want %d", probs, tc.wantProbs)
			}
		})
	}
}

func TestApplyUIOrdering(t *testing.T) {
	rs, err := Compile(Input{
		Redirects: "/x /from-file 301\n",
		Routes:    []Rule{{From: "/x", To: "/from-spec"}},
	}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	rules, blocks, probs := CompileUI([]UIRedirect{{From: "/x", To: "/from-ui"}}, nil, DefaultLimits())
	if len(probs) != 0 {
		t.Fatal(probs)
	}
	yes := true
	for i := 0; i < 2; i++ { // second pass proves ApplyUI is idempotent
		if err := rs.ApplyUI(rules, blocks, OptionOverrides{SPA: &yes, TrailingSlash: SlashAdd}); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"ui", "app.yaml", "file"}
	if len(rs.Rules) != 3 {
		t.Fatalf("rules = %d, want 3", len(rs.Rules))
	}
	for i, w := range want {
		if got := SourceKind(rs.Rules[i].Source); got != w {
			t.Errorf("rule %d source = %s, want %s", i, got, w)
		}
	}
	if !rs.Options.SPA || rs.Options.TrailingSlash != SlashAdd {
		t.Errorf("options = %+v", rs.Options)
	}
	if err := rs.ApplyUI(nil, nil, OptionOverrides{TrailingSlash: "sideways"}); err == nil {
		t.Error("invalid trailing slash accepted")
	}
	_ = rs.ApplyUI(nil, nil, OptionOverrides{})
	if len(rs.Rules) != 2 {
		t.Errorf("clearing UI rules left %d rules, want 2", len(rs.Rules))
	}
}

func TestApplyUICleanURLsOverridesBase(t *testing.T) {
	rs, err := Compile(Input{Options: Options{CleanURLs: true}}, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	off, on := false, true
	if err := rs.ApplyUI(nil, nil, OptionOverrides{CleanURLs: &off}); err != nil || rs.Options.CleanURLs {
		t.Fatalf("explicit off must win: %v %+v", err, rs.Options)
	}
	if err := rs.ApplyUI(nil, nil, OptionOverrides{}); err != nil || !rs.Options.CleanURLs {
		t.Fatalf("auto must fall back to the base: %v %+v", err, rs.Options)
	}
	if err := rs.ApplyUI(nil, nil, OptionOverrides{CleanURLs: &on}); err != nil || !rs.Options.CleanURLs {
		t.Fatalf("explicit on: %v", err)
	}
}
