package main

import (
	"slices"
	"testing"
)

func TestFlagsFirst(t *testing.T) {
	got := flagsFirst([]string{"restart", "api", "--kind", "action", "--json", "--limit=5"})
	want := []string{"--kind", "action", "--json", "--limit=5", "restart", "api"}
	if !slices.Equal(got, want) {
		t.Fatalf("flagsFirst = %v, want %v", got, want)
	}
}
