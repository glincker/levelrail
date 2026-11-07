package main

import (
	"context"
	"strings"
	"testing"
)

func TestVMDestroyIsUninstall(t *testing.T) {
	e, f := runningVM(t)
	if code, _, _ := runVMCmd(e, nil, "destroy"); code != exitUsage || !exists(e.settingsPath) {
		t.Fatal("destroy must need --yes")
	}
	if code, _, errs := runVMCmd(e, nil, "destroy", "--yes"); code != 0 || !f.did("delete --force levelrail") || exists(e.settingsPath) {
		t.Fatalf("%d %s %v", code, errs, f.calls)
	}
}

type missingLima struct{}

func (missingLima) Run(context.Context, ...string) ([]byte, error) { return nil, errLimactlMissing }
func (missingLima) Interactive(context.Context, ...string) error   { return errLimactlMissing }

func TestVMMissingLimactlHint(t *testing.T) {
	e, _ := runningVM(t)
	e.runner = missingLima{}
	for _, sub := range []string{"status", "stop", "shell"} {
		code, _, errs := runVMCmd(e, nil, sub)
		if code != vmExitFail || !strings.Contains(errs, "brew install lima") {
			t.Fatalf("%s: code=%d err=%q", sub, code, errs)
		}
	}
}
