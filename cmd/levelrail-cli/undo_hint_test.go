package main

import "testing"

func TestUndoHintQuotesAndScale(t *testing.T) {
	if got := undoHint("lr", "apps", "rollback", "web", "--image", "reg/app@sha256:ab"); got != "undo: lr apps rollback web --image reg/app@sha256:ab\n" {
		t.Fatalf("got %q", got)
	}
	if got := undoHint("lr", "env", "set", "a", "K=<previous value>"); got != "undo: lr env set a 'K=<previous value>'\n" {
		t.Fatalf("got %q", got)
	}
	got := scaleUndoArgs(appResource{Replicas: 3, Strategy: "rolling"}, 2, "recreate")
	if len(got) != 4 || got[1] != "2" || got[3] != "recreate" {
		t.Fatalf("got %v", got)
	}
	if got := domainUndoArgs("add", "web", "dev", []string{"a.io"}); len(got) != 7 || got[2] != "remove" {
		t.Fatalf("got %v", got)
	}
}
