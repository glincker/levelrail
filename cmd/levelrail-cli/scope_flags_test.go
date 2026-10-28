package main

import (
	"reflect"
	"testing"
)

func TestScopeShorthandFlags(t *testing.T) {
	rest, sc := extractScopeFlags([]string{"-n", "shop", "apps", "list", "-n", "5"})
	if sc.Project != "shop" || !reflect.DeepEqual(rest, []string{"apps", "list", "-n", "5"}) {
		t.Errorf("got %v %+v", rest, sc)
	}
	rest, sc = extractScopeFlags([]string{"apps", "list", "-A"})
	if sc.Project != allProjectsMarker || !reflect.DeepEqual(rest, []string{"apps", "list"}) {
		t.Errorf("got %v %+v", rest, sc)
	}
	rest, _ = extractScopeFlags([]string{"--all-projects", "apps", "list"})
	if !reflect.DeepEqual(rest, []string{"apps", "list"}) {
		t.Errorf("got %v", rest)
	}
}

func TestExpandOutputShorthand(t *testing.T) {
	got := expandOutputShorthand([]string{"apps", "list", "-o", "yaml", "export", "-o", "out.txt", "-o=wide"})
	want := []string{"apps", "list", "--output", "yaml", "export", "-o", "out.txt", "--output=wide"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
