package main

import (
	"reflect"
	"testing"
)

func TestPendingStagesTracksDeferred(t *testing.T) {
	var p pendingStages
	if p.List() != nil {
		t.Fatal("empty tracker must list nothing")
	}
	p.add("heartbeat")
	p.add("monitors")
	p.done("heartbeat")
	if got := p.List(); !reflect.DeepEqual(got, []string{"monitors"}) {
		t.Fatalf("pending = %v", got)
	}
	p.done("monitors")
	if p.List() != nil {
		t.Fatal("tracker must be empty after all stages finish")
	}
}
