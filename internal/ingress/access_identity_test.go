package ingress

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAccessLogIdentityReachesSubscribersOnly(t *testing.T) {
	for _, capacity := range []int{10, 0} {
		l := NewAccessLog(capacity)
		var seen *RequestIdentity
		l.Subscribe(func(_ string, rec AccessRecord) { seen = rec.Identity })
		if l.IdentityWanted() {
			t.Fatal("identity capture must default to off")
		}
		l.CaptureIdentity(true)
		if !l.IdentityWanted() {
			t.Fatal("capture did not switch on")
		}
		l.Record("k", AccessRecord{Time: time.Now(), Path: "/", Status: 200, Identity: &RequestIdentity{ClientIP: "203.0.113.9", UserAgent: "ua"}})
		if seen == nil || seen.ClientIP != "203.0.113.9" {
			t.Errorf("capacity %d: subscriber saw %+v", capacity, seen)
		}
		for _, rec := range l.Query([]string{"k"}, AccessFilter{}) {
			if rec.Identity != nil {
				t.Errorf("capacity %d: the ring stored identity", capacity)
			}
			b, _ := json.Marshal(rec)
			if strings.Contains(string(b), "203.0.113.9") {
				t.Errorf("capacity %d: identity serialized: %s", capacity, b)
			}
		}
	}
}

func TestRequestStatsOwnerOf(t *testing.T) {
	s := NewRequestStats()
	if got := s.OwnerOf("a"); got != "" {
		t.Errorf("OwnerOf before owners are set = %q", got)
	}
	s.SetHostOwners(map[string]string{"a": "web"})
	if got := s.OwnerOf("a"); got != "web" {
		t.Errorf("OwnerOf = %q", got)
	}
	if got := s.OwnerOf("zzz"); got != "" {
		t.Errorf("OwnerOf unknown = %q", got)
	}
}
