package api

import (
	"sync"
	"sync/atomic"
)

// changeTopic names one family of dashboard lists the change stream nudges.
type changeTopic uint8

const (
	changeApps changeTopic = iota
	changeNodes
	changeAlerts
	changeDomains
	changeLoadBalancers
	changeScheduledTasks
	changeCompose
	changeDatabases
	changeIncidents
	changeTopicCount
)

var changeTopicNames = [changeTopicCount]string{"apps", "nodes", "alerts", "domains", "loadbalancers", "scheduled_tasks", "compose", "databases", "incidents"}

// changeEvent carries the resource name only so the stream can check read
// access; the name never leaves the server.
type changeEvent struct {
	topic  changeTopic
	name   string
	detail *changeDetail
}

// changeDetail is the optional payload of a deploy or alert event: ids and
// status only, never env values, args or secrets.
type changeDetail struct {
	Type       string `json:"type"`
	ResourceID string `json:"resource_id"`
	Status     int    `json:"status"`
}

type changeSub struct {
	ch   chan changeEvent
	wake chan struct{}
	// overflow holds one bit per topic dropped while ch was full.
	overflow atomic.Uint32
	caller   string
}

// changeBus fans topic nudges out to live streams. Publish takes no lock
// and never blocks, so write paths pay only an atomic load per call.
type changeBus struct {
	mu      sync.Mutex
	subs    atomic.Pointer[[]*changeSub]
	streams map[string]int
}

func (b *changeBus) Publish(t changeTopic, name string) { b.publish(t, name, nil) }

// PublishDetail is Publish plus a payload that only callers allowed to read
// the named resource receive. A full buffer degrades to the topic nudge.
func (b *changeBus) PublishDetail(t changeTopic, name, eventType string, status int) {
	b.publish(t, name, &changeDetail{Type: eventType, ResourceID: name, Status: status})
}

func (b *changeBus) publish(t changeTopic, name string, d *changeDetail) {
	p := b.subs.Load()
	if p == nil || t >= changeTopicCount {
		return
	}
	for _, s := range *p {
		select {
		case s.ch <- changeEvent{topic: t, name: name, detail: d}:
		default:
			s.overflow.Or(1 << t)
			// The reader may have drained ch just before the Or, so wake it once.
			select {
			case s.wake <- struct{}{}:
			default:
			}
		}
	}
}

// subscribe registers a stream for caller, or returns false when caller
// already holds maxStreams live streams.
func (b *changeBus) subscribe(caller string, buffer, maxStreams int) (*changeSub, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.streams == nil {
		b.streams = map[string]int{}
	}
	if b.streams[caller] >= maxStreams {
		return nil, false
	}
	b.streams[caller]++
	s := &changeSub{ch: make(chan changeEvent, buffer), wake: make(chan struct{}, 1), caller: caller}
	var next []*changeSub
	if p := b.subs.Load(); p != nil {
		next = append(next, *p...)
	}
	next = append(next, s)
	b.subs.Store(&next)
	return s, true
}

func (b *changeBus) unsubscribe(s *changeSub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var next []*changeSub
	if p := b.subs.Load(); p != nil {
		for _, o := range *p {
			if o != s {
				next = append(next, o)
			}
		}
	}
	b.subs.Store(&next)
	if b.streams[s.caller]--; b.streams[s.caller] <= 0 {
		delete(b.streams, s.caller)
	}
}
