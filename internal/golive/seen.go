package golive

import (
	"sync"
	"time"
)

const seenCap = 2000

// Seen remembers when each milestone was first observed, for the lifetime of
// the process, so the timeline keeps its times across page reloads.
type Seen struct {
	mu sync.Mutex
	at map[string]time.Time
}

// NewSeen returns an empty memory.
func NewSeen() *Seen { return &Seen{at: map[string]time.Time{}} }

// Mark records now for key the first time and returns the recorded time.
func (s *Seen) Mark(key string, now time.Time) time.Time {
	if s == nil {
		return now
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.at[key]; ok {
		return t
	}
	if len(s.at) >= seenCap {
		s.at = map[string]time.Time{}
	}
	s.at[key] = now
	return now
}

// Forget drops key, so a milestone that regressed is observed again.
func (s *Seen) Forget(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.at, key)
	s.mu.Unlock()
}

// TimelineEvent is one entry in a domain's go live history.
type TimelineEvent struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Title  string    `json:"title"`
	Detail string    `json:"detail,omitempty"`
	Failed bool      `json:"failed,omitempty"`
	// Future marks something scheduled, such as a renewal.
	Future bool `json:"future,omitempty"`
}

// Timeline event kinds.
const (
	KindDNS       = "dns"
	KindPort      = "port"
	KindChallenge = "challenge"
	KindCert      = "certificate"
	KindRenewal   = "renewal"
	KindFailure   = "failure"
	KindLive      = "live"
)

func buildTimeline(in Input, steps []Step) []TimelineEvent {
	var out []TimelineEvent
	key := in.App + "/" + in.Domain + "/"
	status := map[string]Status{}
	for _, s := range steps {
		status[s.ID] = s.Status
	}
	milestone := func(id string, ok bool, kind, title, detail string) {
		if !ok {
			in.Seen.Forget(key + id)
			return
		}
		out = append(out, TimelineEvent{ID: id, At: in.Seen.Mark(key+id, in.Now), Kind: kind, Title: title, Detail: detail})
	}
	milestone("dns_detected", status[StepDNS] == StatusOK, KindDNS, "DNS points at this server", in.Domain)
	milestone("port_open", status[StepFirewall] == StatusOK, KindPort, "Ports 80 and 443 are open on this server", "")
	if !in.ChallengeStartedAt.IsZero() {
		out = append(out, TimelineEvent{ID: "challenge", At: in.ChallengeStartedAt, Kind: KindChallenge, Title: "Certificate requested", Detail: "Let's Encrypt is validating the domain over port 80"})
	}
	if in.Failure != nil {
		title := "Certificate request failed"
		if in.Failure.Renewal {
			title = "Certificate renewal failed"
		}
		d := in.Failure.Explanation
		if d == "" {
			d = in.Failure.Error
		}
		out = append(out, TimelineEvent{ID: "failure", At: in.Failure.At, Kind: KindFailure, Title: title, Detail: d, Failed: true})
	}
	if c := in.Cert.Status; c.State == "issued" && c.Trusted && !c.NotBefore.IsZero() {
		out = append(out, TimelineEvent{ID: "issued", At: c.NotBefore, Kind: KindCert, Title: "Certificate issued", Detail: c.Issuer})
		if !c.RenewalAt.IsZero() {
			out = append(out, TimelineEvent{ID: "renewal", At: c.RenewalAt, Kind: KindRenewal, Title: "Renewal scheduled", Future: c.RenewalAt.After(in.Now)})
		}
	}
	milestone("live", status[StepLive] == StatusOK, KindLive, "https://"+in.Domain+" serves this app", "")
	sortTimeline(out)
	return out
}

func sortTimeline(ev []TimelineEvent) {
	for i := 1; i < len(ev); i++ {
		for j := i; j > 0 && ev[j].At.Before(ev[j-1].At); j-- {
			ev[j], ev[j-1] = ev[j-1], ev[j]
		}
	}
}
