package ingress

import (
	"fmt"
	"testing"
	"time"
)

func TestReplaceHostOwnersReportsChangedKeys(t *testing.T) {
	s := NewRequestStats()
	if got := s.ReplaceHostOwners(map[string]string{"a.example.com": "alpha", "b.example.com": "beta"}); len(got) != 0 {
		t.Fatalf("first publication changed %v", got)
	}
	got := s.ReplaceHostOwners(map[string]string{"a.example.com": "alpha", "b.example.com": "gamma"})
	if len(got) != 1 || got[0] != "b.example.com" {
		t.Fatalf("changed = %v", got)
	}
	if _, ok := s.OwnerSince()["b.example.com"]; !ok {
		t.Fatal("owner change did not stamp a start time")
	}
	if _, ok := s.OwnerSince()["a.example.com"]; ok {
		t.Fatal("unchanged key got a start time")
	}
	if got := s.ReplaceHostOwners(map[string]string{"a.example.com": "alpha"}); len(got) != 1 || got[0] != "b.example.com" {
		t.Fatalf("removed = %v", got)
	}
}

func TestPurgeAndOwnerSinceHideOldRows(t *testing.T) {
	l := NewAccessLog(10)
	now := time.Now()
	l.Record("k", AccessRecord{Time: now.Add(-time.Minute), Path: "/old", RequestID: "old"})
	l.Record("k", AccessRecord{Time: now, Path: "/new", RequestID: "new"})
	q := AccessSearch{From: now.Add(-time.Hour), Limit: 10}
	res := l.Search([]string{"k"}, q, map[string]time.Time{"k": now.Add(-time.Second)})
	if len(res.Records) != 1 || res.Records[0].RequestID != "new" {
		t.Fatalf("owner start not applied: %+v", res.Records)
	}
	if got := l.Query([]string{"k"}, AccessFilter{KeySince: map[string]time.Time{"k": now.Add(-time.Second)}}); len(got) != 1 {
		t.Fatalf("Query ignored KeySince: %+v", got)
	}
	l.Purge("k")
	if res := l.Search([]string{"k"}, q, nil); len(res.Records) != 0 {
		t.Fatalf("purged ring still returns %d rows", len(res.Records))
	}
	l.mu.RLock()
	n := len(l.rings)
	l.mu.RUnlock()
	if n != 0 {
		t.Fatalf("purge left %d rings", n)
	}
}

func TestSearchPagesAcrossRingsWithScanCap(t *testing.T) {
	l := NewAccessLog(50)
	now := time.Now()
	keys := []string{"c", "a", "b"}
	for i := range 30 {
		k := keys[i%3]
		l.Record(k, AccessRecord{Time: now.Add(-time.Duration(i) * time.Second), Path: "/p", RequestID: fmt.Sprintf("%s-%02d", k, i)})
	}
	seen := map[string]bool{}
	q := AccessSearch{From: now.Add(-time.Hour), Limit: 4, MaxScan: 3}
	for range 40 {
		res := l.Search(keys, q, nil)
		for _, r := range res.Records {
			if seen[r.RequestID] {
				t.Fatalf("row %s returned twice", r.RequestID)
			}
			seen[r.RequestID] = true
		}
		if res.Next == nil {
			break
		}
		q.Before = res.Next
	}
	if len(seen) != 30 {
		t.Fatalf("paged %d rows, want 30", len(seen))
	}
}
