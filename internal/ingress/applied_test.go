package ingress

import (
	"context"
	"testing"
	"time"
)

func TestWaitAppliedNeedsALaterPass(t *testing.T) {
	early := BeginPass()
	mark := PassMark()
	EndPass(early)
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if WaitApplied(short, mark) {
		t.Fatal("a pass that started before the mark must not count")
	}
	later := BeginPass()
	go EndPass(later)
	long, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if !WaitApplied(long, mark) {
		t.Fatal("a later applied pass must release the wait")
	}
}
