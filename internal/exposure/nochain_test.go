package exposure

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func nftManager(out string) *Manager {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(out), errors.New("exit status 1")
	}
	return NewFakeManager(testPrefix, nil, run, func(string) (string, error) { return "/sbin/iptables", nil }, "linux")
}

func TestMissingChainIsNftablesNotAnError(t *testing.T) {
	m := nftManager("iptables: No chain/target/match by that name.\n")
	c := m.ReadChain(context.Background())
	if c.Readable || !c.NoChain || !strings.Contains(c.Reason, "uses nftables") {
		t.Fatalf("chain = %+v", c)
	}
	if res := m.Sync(context.Background(), nil); len(res.Errors) != 0 {
		t.Fatalf("empty sync reported errors: %v", res.Errors)
	}
	res := m.Sync(context.Background(), []Restriction{{Port: 8108, Allow: []string{"10.0.0.0/8"}}})
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "uses nftables") {
		t.Fatalf("sync with restrictions = %+v", res)
	}
}

func TestOtherReadFailureIsNotNoChain(t *testing.T) {
	m := nftManager("Permission denied (you must be root)\n")
	if c := m.ReadChain(context.Background()); c.NoChain || c.Readable {
		t.Fatalf("chain = %+v", c)
	}
}
