package main

import (
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestWaitPatchRunTreatsMissingOrErrorAsUnknown(t *testing.T) {
	running := apiclient.NodePatchRun{ID: "r1", Status: "running"}
	noSleep := func() {}
	cases := map[string]func() ([]apiclient.NodePatchRun, error){
		"vanished":   func() ([]apiclient.NodePatchRun, error) { return nil, nil },
		"list error": func() ([]apiclient.NodePatchRun, error) { return nil, errors.New("connection refused") },
	}
	for name, list := range cases {
		if got := waitPatchRun(list, running, noSleep); got.Status != patchStatusUnknown {
			t.Errorf("%s: status = %q, want %q", name, got.Status, patchStatusUnknown)
		}
	}
	done := func() ([]apiclient.NodePatchRun, error) {
		return []apiclient.NodePatchRun{{ID: "r1", Status: "succeeded"}}, nil
	}
	if got := waitPatchRun(done, running, noSleep); got.Status != "succeeded" {
		t.Errorf("finished run status = %q", got.Status)
	}
}
