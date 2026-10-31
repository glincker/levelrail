package spec

import (
	"strings"
	"testing"
)

func TestScaffoldValidates(t *testing.T) {
	cases := []ScaffoldOptions{
		{},
		{Name: "api", Build: "railpack", Port: 3000, Domain: "api.example.com", DB: "postgres"},
		{Name: "site", Image: "nginx:1.27", Port: 80},
		{Name: "docs", Build: "static"},
	}
	for _, c := range cases {
		out, err := Scaffold(c)
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		if _, err := Parse(out); err != nil {
			t.Fatalf("%+v does not parse: %v\n%s", c, err, out)
		}
	}
}

func TestScaffoldCommentsComeFromSchema(t *testing.T) {
	out, err := Scaffold(ScaffoldOptions{DB: "redis"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Optional service fields", "#     env:", "#     replicas:", "#     backup:"} {
		if !strings.Contains(string(out), strings.Replace(want, "#     ", "    # ", 1)) && !strings.Contains(string(out), want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestScaffoldRejectsBadInput(t *testing.T) {
	for _, c := range []ScaffoldOptions{{Name: "Bad Name"}, {Port: 70000}, {Build: "nope"}, {Build: "compose"}, {DB: "oracle"}} {
		if _, err := Scaffold(c); err == nil {
			t.Errorf("%+v: want error", c)
		}
	}
}
