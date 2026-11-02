package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestCheckMarkShowsRunningInsteadOfFail(t *testing.T) {
	if got := checkMark("running", false); got != "running" {
		t.Errorf("running = %q", got)
	}
	if got := checkMark("failed", false); got != "FAIL" {
		t.Errorf("failed = %q", got)
	}
	if got := checkMark("passed", true); got != "ok" {
		t.Errorf("passed = %q", got)
	}
}

func TestValidatePlanVolumesRejectsHostPathBeforeCreate(t *testing.T) {
	err := validatePlanVolumes([]spec.Volume{{HostPath: "/srv/cfg", Path: "/x"}})
	if err == nil {
		t.Fatal("expected an error for a bind mount")
	}
	if validatePlanVolumes([]spec.Volume{{Name: "data", Path: "/d"}}) != nil {
		t.Fatal("named volume must pass")
	}
}
