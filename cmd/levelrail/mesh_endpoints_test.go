package main

import (
	"log/slog"
	"testing"
)

type fakeRecorder map[string]string

func (f fakeRecorder) SetObservedEndpoint(id, ep string) error { f[id] = ep; return nil }

func TestMeshEndpointBook(t *testing.T) {
	b := newMeshEndpointBook(slog.Default())
	b.Observe("w1", "203.0.113.7")
	b.Observe("w2", "127.0.0.1")
	b.Observe("hub", "198.51.100.1")
	rec := fakeRecorder{}
	b.Attach(rec, "hub")
	b.Observe("w3", "2001:db8::5")
	b.Observe("hub", "198.51.100.1")

	want := map[string]string{"w1": "203.0.113.7:51820", "w3": "[2001:db8::5]:51820"}
	if len(rec) != len(want) {
		t.Fatalf("recorded %v, want %v", rec, want)
	}
	for id, ep := range want {
		if rec[id] != ep {
			t.Fatalf("recorded[%s] = %q, want %q", id, rec[id], ep)
		}
	}
}
