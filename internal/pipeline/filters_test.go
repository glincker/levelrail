package pipeline

import (
	"errors"
	"reflect"
	"testing"
)

const filtersBase = `version: 1
name: ci
on:
  push:
    branches: [main]
  pull_request:
jobs:
  test:
    image: golang:1.22
    steps:
      - run: go test ./...
`

func TestApplyFiltersRoundTrip(t *testing.T) {
	off := false
	out, err := ApplyFilters([]byte(filtersBase), []string{"src/**", "go.mod"}, []string{"**/*.md"}, &off)
	if err != nil {
		t.Fatal(err)
	}
	def, issues := Validate(out)
	if len(issues) > 0 {
		t.Fatalf("result invalid: %v\n%s", issues, out)
	}
	if !reflect.DeepEqual([]string(def.On.Push.Paths), []string{"src/**", "go.mod"}) || !reflect.DeepEqual([]string(def.On.Push.PathsIgnore), []string{"**/*.md"}) {
		t.Errorf("push filters = %v / %v", def.On.Push.Paths, def.On.Push.PathsIgnore)
	}
	if !reflect.DeepEqual([]string(def.On.PullRequest.Paths), []string{"src/**", "go.mod"}) {
		t.Errorf("a null pull_request trigger did not receive paths: %v", def.On.PullRequest.Paths)
	}
	if def.ReportsStatus() {
		t.Error("report_status = true, want false")
	}
	if def.On.Push.Branches[0] != "main" {
		t.Errorf("existing branches lost: %v", def.On.Push.Branches)
	}
}

func TestApplyFiltersClearAndKeep(t *testing.T) {
	first, err := ApplyFilters([]byte(filtersBase), []string{"src/**"}, []string{"docs/**"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := ApplyFilters(first, nil, []string{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	def, issues := Validate(kept)
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	v := FiltersOf(def)
	if !reflect.DeepEqual(v.Paths, []string{"src/**"}) || len(v.PathsIgnore) != 0 || !v.ReportStatus {
		t.Errorf("FiltersOf = %+v, want paths kept, ignore cleared, report on", v)
	}
}

func TestApplyFiltersNoTrigger(t *testing.T) {
	src := "version: 1\nname: x\non:\n  manual:\njobs:\n  a:\n    steps:\n      - run: echo\n"
	if _, err := ApplyFilters([]byte(src), []string{"a"}, nil, nil); !errors.Is(err, ErrNoPathTrigger) {
		t.Fatalf("err = %v, want ErrNoPathTrigger", err)
	}
	if _, err := ApplyFilters([]byte(src), nil, nil, nil); err != nil {
		t.Fatalf("no-op apply failed: %v", err)
	}
}
