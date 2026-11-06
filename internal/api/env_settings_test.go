package api

import (
	"reflect"
	"testing"
)

func TestBuildTimeEnvChanges(t *testing.T) {
	t.Parallel()
	prev := map[string]string{"NEXT_PUBLIC_A": "1", "VITE_B": "1", "PLAIN": "1", "NEXT_PUBLIC_SAME": "1"}
	next := map[string]string{"NEXT_PUBLIC_A": "2", "PLAIN": "2", "NEXT_PUBLIC_NEW": "x", "NEXT_PUBLIC_SAME": "1"}
	got := buildTimeEnvChanges(prev, next)
	want := []string{"NEXT_PUBLIC_A", "NEXT_PUBLIC_NEW", "VITE_B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(buildTimeEnvChanges(prev, prev)) != 0 {
		t.Error("no change must report nothing")
	}
}
