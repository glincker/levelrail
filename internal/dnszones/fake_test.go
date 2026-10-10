package dnszones

import (
	"context"
	"slices"
	"sync"

	"github.com/miekg/dns"
)

// fakeProvider is an in-memory Provider for plan tests.
type fakeProvider struct {
	caps  Capabilities
	sets  []RecordSet
	calls []string
}

func (f *fakeProvider) Name() string                                     { return "fake" }
func (f *fakeProvider) Capabilities() Capabilities                       { return f.caps }
func (f *fakeProvider) ListZones(context.Context) ([]Zone, error)        { return nil, nil }
func (f *fakeProvider) GetZone(context.Context, string) (Zone, error)    { return Zone{}, nil }
func (f *fakeProvider) CreateZone(context.Context, string) (Zone, error) { return Zone{}, nil }
func (f *fakeProvider) DeleteZone(context.Context, string) error         { return nil }
func (f *fakeProvider) ListRecordSets(context.Context, Zone) ([]RecordSet, error) {
	return slices.Clone(f.sets), nil
}

func (f *fakeProvider) UpsertRecordSet(_ context.Context, _ Zone, rs RecordSet) error {
	f.calls = append(f.calls, "upsert "+rs.Name+" "+rs.Type)
	f.sets = slices.DeleteFunc(f.sets, func(e RecordSet) bool { return e.Key() == rs.Key() })
	f.sets = append(f.sets, rs)
	return nil
}

func (f *fakeProvider) DeleteRecordSet(_ context.Context, _ Zone, k Key) error {
	f.calls = append(f.calls, "delete "+k.Name+" "+k.Type)
	f.sets = slices.DeleteFunc(f.sets, func(e RecordSet) bool { return e.Key() == k })
	return nil
}

// fakeQuerier answers from a table keyed by server, name and type.
type fakeQuerier struct {
	mu      sync.Mutex
	answers map[string]Answer
	asked   []string
}

func qkey(server, name string, t uint16) string {
	return server + "|" + NormalizeDomain(name) + "|" + dns.TypeToString[t]
}

func (f *fakeQuerier) Query(_ context.Context, server, name string, t uint16) Answer {
	k := qkey(server, name, t)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, k)
	if a, ok := f.answers[k]; ok {
		a.Server = server
		return a
	}
	return Answer{Server: server, Rcode: "nxdomain"}
}
