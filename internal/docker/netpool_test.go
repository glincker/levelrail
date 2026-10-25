package docker

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	dockernetwork "github.com/docker/docker/api/types/network"
)

func pfx(s string) netip.Prefix { return netip.MustParsePrefix(s) }

func TestParseNetworkPool(t *testing.T) {
	tests := []struct {
		in      string
		ok      bool
		wantErr bool
		cap     int
	}{
		{"", true, false, 1024},
		{"10.99.0.0/16", true, false, 256},
		{"off", false, false, 0},
		{"10.99.0.1/16", false, true, 0},
		{"10.99.0.0/25", false, true, 0},
		{"fd00::/48", false, true, 0},
		{"junk", false, true, 0},
	}
	for _, tc := range tests {
		p, ok, err := ParseNetworkPool(tc.in)
		if (err != nil) != tc.wantErr || ok != tc.ok {
			t.Fatalf("%q: ok=%v err=%v", tc.in, ok, err)
		}
		if ok && p.Capacity() != tc.cap {
			t.Errorf("%q: capacity %d want %d", tc.in, p.Capacity(), tc.cap)
		}
	}
}

func TestPoolAllocateAvoidsPodmanDefault(t *testing.T) {
	p, _, _ := ParseNetworkPool("")
	got, ok := p.Allocate(nil)
	if !ok || got.String() != "10.89.0.0/24" {
		t.Fatalf("got %v %v, want 10.89.0.0/24", got, ok)
	}
}

func TestPoolAllocate(t *testing.T) {
	p, _, _ := ParseNetworkPool("10.99.0.0/22")
	tests := []struct {
		name string
		used []netip.Prefix
		want string
		ok   bool
	}{
		{"empty", nil, "10.99.0.0/24", true},
		{"skips used", []netip.Prefix{pfx("10.99.0.0/24"), pfx("10.99.1.0/24")}, "10.99.2.0/24", true},
		{"skips overlapping wider block", []netip.Prefix{pfx("10.99.0.0/23")}, "10.99.2.0/24", true},
		{"skips host route inside", []netip.Prefix{pfx("10.99.0.128/25")}, "10.99.1.0/24", true},
		{"exhausted", []netip.Prefix{pfx("10.99.0.0/22")}, "", false},
	}
	for _, tc := range tests {
		got, ok := p.Allocate(tc.used)
		if ok != tc.ok || (ok && got.String() != tc.want) {
			t.Errorf("%s: got %v %v want %v %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

type fakeNetAPI struct {
	existing  []dockernetwork.Summary
	createErr []error
	created   []string
}

func (f *fakeNetAPI) NetworkList(context.Context, dockernetwork.ListOptions) ([]dockernetwork.Summary, error) {
	return f.existing, nil
}

func (f *fakeNetAPI) NetworkCreate(_ context.Context, _ string, o dockernetwork.CreateOptions) (dockernetwork.CreateResponse, error) {
	if len(f.createErr) > 0 {
		err := f.createErr[0]
		f.createErr = f.createErr[1:]
		if err != nil {
			return dockernetwork.CreateResponse{}, err
		}
	}
	f.created = append(f.created, o.IPAM.Config[0].Subnet)
	return dockernetwork.CreateResponse{ID: "id"}, nil
}

func summary(subnet string) dockernetwork.Summary {
	return dockernetwork.Summary{IPAM: dockernetwork.IPAM{Config: []dockernetwork.IPAMConfig{{Subnet: subnet}}}}
}

func withNoHostPrefixes(t *testing.T) {
	old := hostPrefixes
	hostPrefixes = func() []netip.Prefix { return []netip.Prefix{pfx("192.168.1.0/24")} }
	t.Cleanup(func() { hostPrefixes = old })
}

func TestAllocatorCreate(t *testing.T) {
	withNoHostPrefixes(t)
	pool, _, _ := ParseNetworkPool("10.99.0.0/22")
	a := &poolAllocator{}

	t.Run("picks next free", func(t *testing.T) {
		api := &fakeNetAPI{existing: []dockernetwork.Summary{summary("10.99.0.0/24"), summary("172.17.0.0/16")}}
		if _, err := a.create(context.Background(), api, pool, "n", nil); err != nil {
			t.Fatal(err)
		}
		if len(api.created) != 1 || api.created[0] != "10.99.1.0/24" {
			t.Fatalf("created %v", api.created)
		}
	})

	t.Run("retries on overlap", func(t *testing.T) {
		api := &fakeNetAPI{createErr: []error{errors.New("Pool overlaps with other one on this address space"), nil}}
		if _, err := a.create(context.Background(), api, pool, "n", nil); err != nil {
			t.Fatal(err)
		}
		if len(api.created) != 1 || api.created[0] != "10.99.1.0/24" {
			t.Fatalf("created %v", api.created)
		}
	})

	t.Run("bounded retries", func(t *testing.T) {
		errs := make([]error, 20)
		for i := range errs {
			errs[i] = errors.New("Pool overlaps with other one on this address space")
		}
		api := &fakeNetAPI{createErr: errs}
		big, _, _ := ParseNetworkPool("10.98.0.0/20")
		_, err := a.create(context.Background(), api, big, "n", nil)
		var ex *PoolExhaustedError
		if !errors.As(err, &ex) {
			t.Fatalf("want PoolExhaustedError, got %v", err)
		}
		if left := len(errs) - len(api.createErr); left != maxAllocAttempts {
			t.Errorf("attempts %d want %d", left, maxAllocAttempts)
		}
	})

	t.Run("exhausted reports numbers and fix", func(t *testing.T) {
		api := &fakeNetAPI{existing: []dockernetwork.Summary{summary("10.99.0.0/22")}}
		_, err := a.create(context.Background(), api, pool, "n", nil)
		var ex *PoolExhaustedError
		if !errors.As(err, &ex) || ex.Capacity != 4 || ex.InUse != 1 {
			t.Fatalf("got %v", err)
		}
		if !strings.Contains(err.Error(), EnvNetworkPool) || !strings.Contains(err.Error(), "4") {
			t.Errorf("message lacks fix: %v", err)
		}
	})

	t.Run("other errors pass through", func(t *testing.T) {
		boom := errors.New("daemon down")
		api := &fakeNetAPI{createErr: []error{boom}}
		if _, err := a.create(context.Background(), api, pool, "n", nil); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestProcRoutePrefixes(t *testing.T) {
	if got := procRoutePrefixes("/nonexistent/route"); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestPoolExhaustedDaemonMessage(t *testing.T) {
	if !IsPoolExhausted(errors.New("Error response from daemon: all predefined address pools have been fully subnetted")) {
		t.Fatal("not detected")
	}
	msg := (&PoolExhaustedError{Daemon: true, InUse: 31}).Error()
	if !strings.Contains(msg, "daemon.json") || !strings.Contains(msg, "31") {
		t.Errorf("msg %q", msg)
	}
}
