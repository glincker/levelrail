package api

import (
	"strconv"
	"testing"
	"time"
)

func newTestVerifyLimiter() *verifyLimiter {
	return &verifyLimiter{lastActor: map[string]time.Time{}, lastDomain: map[string]time.Time{}, inflight: map[string]int{}}
}

func TestVerifyLimiter_EvictsOldestNotEverything(t *testing.T) {
	l := newTestVerifyLimiter()
	base := time.Now()
	for i := 0; i < verifyLimiterMax; i++ {
		rel, _ := l.acquire("a"+strconv.Itoa(i), "d"+strconv.Itoa(i)+".test", time.Hour, base.Add(time.Duration(i)*time.Second))
		if rel == nil {
			t.Fatalf("acquire %d refused", i)
		}
		rel()
	}
	now := base.Add(time.Duration(verifyLimiterMax) * time.Second)
	if rel, _ := l.acquire("new", "new.test", time.Hour, now); rel == nil {
		t.Fatal("new key refused")
	} else {
		rel()
	}
	if len(l.lastActor) != verifyLimiterMax {
		t.Errorf("lastActor size = %d, want %d", len(l.lastActor), verifyLimiterMax)
	}
	// The newest earlier actor keeps its spacing; the oldest was evicted.
	last := "a" + strconv.Itoa(verifyLimiterMax-1)
	if rel, _ := l.acquire(last, "d"+strconv.Itoa(verifyLimiterMax-1)+".test", time.Hour, now); rel != nil {
		t.Error("recent actor lost its spacing record")
	}
	if _, ok := l.lastActor["a0|d0.test"]; ok {
		t.Error("oldest record was not evicted")
	}
}

func TestVerifyLimiter_PerActorInflightCap(t *testing.T) {
	l := newTestVerifyLimiter()
	now := time.Now()
	var rels []func()
	for i := 0; i < verifyMaxPerActor; i++ {
		rel, _ := l.acquire("greedy", "d"+strconv.Itoa(i)+".test", time.Second, now)
		if rel == nil {
			t.Fatalf("acquire %d refused", i)
		}
		rels = append(rels, rel)
	}
	if rel, _ := l.acquire("greedy", "extra.test", time.Second, now); rel != nil {
		t.Fatal("actor exceeded its in-flight cap")
	}
	if rel, _ := l.acquire("other", "other.test", time.Second, now); rel == nil {
		t.Fatal("another actor was starved by the greedy one")
	} else {
		rel()
	}
	rels[0]()
	if rel, _ := l.acquire("greedy", "again.test", time.Second, now); rel == nil {
		t.Fatal("slot not freed on release")
	}
}
