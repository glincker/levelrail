package main

import "testing"

func TestCleanCell(t *testing.T) {
	if got := cleanCell("a\x1b[31mb\nc"); got != "a [31mb c" {
		t.Fatalf("got %q", got)
	}
	long := ""
	for range 50 {
		long += "é"
	}
	if got := cleanCell(long); len([]rune(got)) != 43 {
		t.Fatalf("got %d runes", len([]rune(got)))
	}
}
