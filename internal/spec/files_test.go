package spec

import "testing"

func TestValidateFiles(t *testing.T) {
	lim := FileLimits{MaxBytes: 10, MaxTotal: 15, MaxCount: 3}
	cases := []struct {
		name  string
		files []File
		ok    bool
	}{
		{"fine", []File{{Path: "/etc/a.conf", Content: "x"}}, true},
		{"relative", []File{{Path: "a.conf"}}, false},
		{"dotdot", []File{{Path: "/etc/../a"}}, false},
		{"proc", []File{{Path: "/proc/x"}}, false},
		{"dup", []File{{Path: "/a"}, {Path: "/a"}}, false},
		{"bad mode", []File{{Path: "/a", Mode: "999"}}, false},
		{"too big", []File{{Path: "/a", Content: "12345678901"}}, false},
		{"total", []File{{Path: "/a", Content: "1234567890"}, {Path: "/b", Content: "123456"}}, false},
	}
	for _, c := range cases {
		if err := ValidateFiles(c.files, lim); (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
}

func TestExpandFilePlaceholders(t *testing.T) {
	got, err := ExpandFilePlaceholders("a=${K} $$x", map[string]string{"K": "v"})
	if err != nil || got != "a=v $x" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := ExpandFilePlaceholders("${NOPE}", nil); err == nil {
		t.Fatal("want error")
	}
}
