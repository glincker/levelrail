package main

import (
	"reflect"
	"testing"
)

func TestPagerWantedAndFlag(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	ci := func(k string) (string, bool) { return "1", k == "CI" }
	if !pagerWanted([]string{"apps", "list"}, false, true, none) {
		t.Error("tty list should page")
	}
	cases := map[string]bool{
		"piped":  pagerWanted([]string{"apps", "list"}, false, false, none),
		"ci":     pagerWanted([]string{"apps", "list"}, false, true, ci),
		"flag":   pagerWanted([]string{"apps", "list"}, true, true, none),
		"follow": pagerWanted([]string{"apps", "logs", "-f"}, false, true, none),
		"exec":   pagerWanted([]string{"exec", "x"}, false, true, none),
	}
	for name, got := range cases {
		if got {
			t.Errorf("%s should not page", name)
		}
	}
	rest, set := extractNoPagerFlag([]string{"apps", "--no-pager", "list"})
	if !set || !reflect.DeepEqual(rest, []string{"apps", "list"}) {
		t.Errorf("rest=%v set=%v", rest, set)
	}
	if got := pagerCommand("lrtest-nopager", func(string) (string, bool) { return "", false }); !reflect.DeepEqual(got, []string{"less", "-FRX"}) {
		t.Errorf("default pager = %v", got)
	}
	if got := pagerCommand("lrtest-nopager", func(k string) (string, bool) { return "off", k == "PAGER" }); got != nil {
		t.Errorf("off pager = %v", got)
	}
}
