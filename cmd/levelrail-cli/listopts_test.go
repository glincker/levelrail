package main

import "testing"

func TestListOptsQuery(t *testing.T) {
	o := &listOpts{status: "failed", sort: "-updated", limit: 5}
	if got, want := o.query(), "limit=5&sort=-updated&status=failed"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestListOptsApply(t *testing.T) {
	type app struct {
		Name      string `json:"name"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
	}
	in := []app{
		{"a", "failed", "2026-01-01"}, {"b", "running", "2026-01-05"},
		{"c", "Failed", "2026-01-09"}, {"d", "failed", "2026-01-03"},
	}
	got := applyListOpts(in, &listOpts{status: "failed", sort: "-updated", limit: 2})
	if len(got) != 2 || got[0].Name != "c" || got[1].Name != "d" {
		t.Fatalf("got %+v, want c then d", got)
	}
}
