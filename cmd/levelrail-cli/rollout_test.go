package main

import (
	"bytes"
	"testing"
)

func TestRolloutRoutesAndRejects(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runRollout("lr", []string{"pause"}, &out, &errb, nil); code != exitUsage {
		t.Errorf("pause without an app: code %d, %q", code, errb.String())
	}
	errb.Reset()
	if code := runRollout("lr", []string{"bogus"}, &out, &errb, nil); code != exitUsage {
		t.Errorf("bogus code %d", code)
	}
	for verb, want := range map[string]string{"status": "wait", "history": "deploys", "undo": "rollback", "restart": "restart"} {
		if got := rolloutVerbs[verb][0]; got != want {
			t.Errorf("%s -> %s, want %s", verb, got, want)
		}
	}
	out.Reset()
	if code := runRollout("lr", []string{"history", "--help"}, &out, &errb, nil); code != exitOK {
		t.Errorf("history --help code %d", code)
	}
}
