package network

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestEncodeUAPI(t *testing.T) {
	priv := testKey(100)
	peerA := testKey(1)
	peerB := testKey(2)

	cfg := DeviceConfig{
		NodeID:     "self",
		PrivateKey: priv,
		Address:    netip.MustParsePrefix("10.181.0.1/16"),
		ListenPort: 51820,
		Peers: []PeerConfig{
			{
				NodeID:              "a",
				PublicKey:           peerA,
				Endpoint:            "203.0.113.2:51820",
				AllowedIPs:          []netip.Prefix{netip.MustParsePrefix("10.181.0.2/32")},
				PersistentKeepalive: 25 * time.Second,
			},
			{
				NodeID:     "b",
				PublicKey:  peerB,
				AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.181.0.3/32")},
			},
		},
	}

	got := EncodeUAPI(cfg, nil)

	wantLines := []string{
		"private_key=" + priv.Hex(),
		"listen_port=51820",
		"public_key=" + peerA.Hex(),
		"endpoint=203.0.113.2:51820",
		"persistent_keepalive_interval=25",
		"replace_allowed_ips=true",
		"allowed_ip=10.181.0.2/32",
		"public_key=" + peerB.Hex(),
		"persistent_keepalive_interval=0",
		"allowed_ip=10.181.0.3/32",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("payload missing %q\ngot:\n%s", want, got)
		}
	}

	// A peer with no endpoint must not emit an empty one: "endpoint="
	// would be a parse error at the device, not an unset field.
	if strings.Contains(got, "endpoint=\n") {
		t.Errorf("emitted an empty endpoint line\ngot:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Error("payload is not terminated by a blank line")
	}
}

// TestEncodeUAPI_ConvergesWithoutReplacePeers is the property that keeps
// a repeated apply cheap: peers that should stay are upserted rather than
// torn down, so their established sessions survive, and only peers that
// are genuinely gone get a remove.
func TestEncodeUAPI_ConvergesWithoutReplacePeers(t *testing.T) {
	keep := testKey(1)
	gone := testKey(2)
	added := testKey(3)

	cfg := DeviceConfig{
		NodeID: "self",
		Peers: []PeerConfig{
			{NodeID: "keep", PublicKey: keep, AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.181.0.2/32")}},
			{NodeID: "added", PublicKey: added, AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.181.0.4/32")}},
		},
	}

	got := EncodeUAPI(cfg, []Key{keep, gone})

	if strings.Contains(got, "replace_peers") {
		t.Error("payload uses replace_peers, which discards every established session on every apply")
	}
	if !strings.Contains(got, "public_key="+gone.Hex()+"\nremove=true\n") {
		t.Errorf("the departed peer was not removed\ngot:\n%s", got)
	}
	if strings.Contains(got, "public_key="+keep.Hex()+"\nremove=true") {
		t.Errorf("a peer that should stay was removed\ngot:\n%s", got)
	}
	if !strings.Contains(got, "public_key="+added.Hex()) {
		t.Errorf("the new peer was not added\ngot:\n%s", got)
	}
}

func TestEncodeUAPI_OmitsUnsetPrivateKeyAndPort(t *testing.T) {
	// A config that crossed the wire from the control plane has no
	// private key. Writing 32 zero bytes would tell the device to discard
	// its identity, which is the opposite of "not changing this field".
	got := EncodeUAPI(DeviceConfig{NodeID: "self"}, nil)
	if strings.Contains(got, "private_key=") {
		t.Errorf("emitted a private_key line for a zero key\ngot:\n%s", got)
	}
	if strings.Contains(got, "listen_port=") {
		t.Errorf("emitted a listen_port line for an unset port\ngot:\n%s", got)
	}
}

func TestParseUAPIStatus(t *testing.T) {
	priv := testKey(100)
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	peerA, peerB := testKey(1), testKey(2)

	raw := strings.Join([]string{
		"private_key=" + priv.Hex(),
		"listen_port=51820",
		"public_key=" + peerA.Hex(),
		"endpoint=203.0.113.2:51820",
		"allowed_ip=10.181.0.2/32",
		"last_handshake_time_sec=1700000000",
		"last_handshake_time_nsec=500",
		"rx_bytes=1024",
		"tx_bytes=2048",
		"public_key=" + peerB.Hex(),
		"allowed_ip=10.181.0.3/32",
		"last_handshake_time_sec=0",
		"last_handshake_time_nsec=0",
		"rx_bytes=0",
		"tx_bytes=0",
		"errno=0",
		"",
	}, "\n")

	st, err := ParseUAPIStatus(raw, map[Key]string{peerA: "node-a", peerB: "node-b"})
	if err != nil {
		t.Fatalf("ParseUAPIStatus: %v", err)
	}

	if st.PublicKey != pub {
		t.Error("device public key was not derived from the reported private key")
	}
	if st.ListenPort != 51820 {
		t.Errorf("ListenPort = %d, want 51820", st.ListenPort)
	}
	if len(st.Peers) != 2 {
		t.Fatalf("got %d peers, want 2", len(st.Peers))
	}

	// Field attribution across peer boundaries is the thing a naive
	// parser gets wrong: peer A's counters must not land on peer B.
	a := st.Peers[0]
	if a.NodeID != "node-a" {
		t.Errorf("peer 0 NodeID = %q, want node-a", a.NodeID)
	}
	if a.Endpoint != "203.0.113.2:51820" {
		t.Errorf("peer 0 Endpoint = %q", a.Endpoint)
	}
	if a.TransferRx != 1024 || a.TransferTx != 2048 {
		t.Errorf("peer 0 counters = rx %d tx %d, want 1024/2048", a.TransferRx, a.TransferTx)
	}
	if a.LastHandshake.Unix() != 1700000000 {
		t.Errorf("peer 0 LastHandshake = %v", a.LastHandshake)
	}

	b := st.Peers[1]
	if b.NodeID != "node-b" {
		t.Errorf("peer 1 NodeID = %q, want node-b", b.NodeID)
	}
	if b.Endpoint != "" {
		t.Errorf("peer 1 inherited peer 0's endpoint: %q", b.Endpoint)
	}
	if b.TransferRx != 0 || b.TransferTx != 0 {
		t.Errorf("peer 1 inherited peer 0's counters: rx %d tx %d", b.TransferRx, b.TransferTx)
	}
	// A zero handshake time is the "configured but never reachable"
	// state, which must stay zero rather than becoming the Unix epoch.
	if !b.LastHandshake.IsZero() {
		t.Errorf("peer 1 LastHandshake = %v, want the zero time", b.LastHandshake)
	}
}

func TestParseUAPIStatus_UnknownPeerHasNoNodeID(t *testing.T) {
	stray := testKey(9)
	raw := "public_key=" + stray.Hex() + "\nerrno=0\n\n"

	st, err := ParseUAPIStatus(raw, map[Key]string{testKey(1): "node-a"})
	if err != nil {
		t.Fatalf("ParseUAPIStatus: %v", err)
	}
	if len(st.Peers) != 1 {
		t.Fatalf("got %d peers, want 1", len(st.Peers))
	}
	if st.Peers[0].NodeID != "" {
		t.Errorf("NodeID = %q, want empty for a peer no plan accounts for", st.Peers[0].NodeID)
	}
}

func TestParseUAPIStatus_Failures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "malformed line", raw: "this is not a key value line\n", want: "malformed line"},
		{name: "bad listen port", raw: "listen_port=not-a-number\n", want: "listen_port"},
		{name: "bad public key", raw: "public_key=zzzz\n", want: "public_key"},
		{name: "bad private key", raw: "private_key=zzzz\n", want: "private_key"},
		{name: "bad handshake seconds", raw: "public_key=" + testKey(1).Hex() + "\nlast_handshake_time_sec=x\n", want: "last_handshake_time_sec"},
		{name: "bad rx bytes", raw: "public_key=" + testKey(1).Hex() + "\nrx_bytes=x\n", want: "rx_bytes"},
		{name: "device reported an error", raw: "errno=13\n", want: "errno=13"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseUAPIStatus(tc.raw, nil)
			if err == nil {
				t.Fatalf("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseUAPIStatus_IgnoresUnknownKeys(t *testing.T) {
	// A WireGuard upgrade that adds a field must not break status
	// reporting.
	raw := "listen_port=51820\nprotocol_version=1\nfwmark=0\nsomething_new=42\nerrno=0\n\n"
	st, err := ParseUAPIStatus(raw, nil)
	if err != nil {
		t.Fatalf("ParseUAPIStatus: %v", err)
	}
	if st.ListenPort != 51820 {
		t.Errorf("ListenPort = %d, want 51820", st.ListenPort)
	}
}

// TestUAPIRoundTrip checks that what EncodeUAPI writes is the same shape
// ParseUAPIStatus reads back, which is the invariant that keeps Apply's
// read-diff-write cycle correct.
func TestUAPIRoundTrip(t *testing.T) {
	peer := testKey(1)
	cfg := DeviceConfig{
		NodeID:     "self",
		ListenPort: 51820,
		Peers: []PeerConfig{{
			NodeID:     "a",
			PublicKey:  peer,
			Endpoint:   "203.0.113.2:51820",
			AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.181.0.2/32")},
		}},
	}

	// A device echoes back what it was set to, plus errno.
	echoed := EncodeUAPI(cfg, nil) + "errno=0\n"
	st, err := ParseUAPIStatus(echoed, map[Key]string{peer: "a"})
	if err != nil {
		t.Fatalf("ParseUAPIStatus of an encoded config: %v", err)
	}
	if st.ListenPort != cfg.ListenPort {
		t.Errorf("ListenPort round trip: got %d, want %d", st.ListenPort, cfg.ListenPort)
	}
	keys := peerKeys(st)
	if len(keys) != 1 || keys[0] != peer {
		t.Errorf("peerKeys = %v, want [%v]", keys, peer)
	}
}

func TestPeerStatus_Healthy(t *testing.T) {
	now := time.Unix(1700000000, 0)
	tests := []struct {
		name string
		peer PeerStatus
		want bool
	}{
		{name: "never handshaken is unreachable", peer: PeerStatus{}, want: false},
		{name: "just handshaken", peer: PeerStatus{LastHandshake: now}, want: true},
		{name: "within the window", peer: PeerStatus{LastHandshake: now.Add(-2 * time.Minute)}, want: true},
		{name: "past the window", peer: PeerStatus{LastHandshake: now.Add(-10 * time.Minute)}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.peer.Healthy(now, staleAfter); got != tc.want {
				t.Errorf("Healthy = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUnhealthyPeers(t *testing.T) {
	now := time.Unix(1700000000, 0)
	st := Status{Peers: []PeerStatus{
		{NodeID: "fresh", LastHandshake: now},
		{NodeID: "never"},
		{NodeID: "stale", LastHandshake: now.Add(-time.Hour)},
	}}

	got := UnhealthyPeers(st, now)
	if len(got) != 2 {
		t.Fatalf("got %d unhealthy peers, want 2: %+v", len(got), got)
	}
	if got[0].NodeID != "never" || got[1].NodeID != "stale" {
		t.Errorf("UnhealthyPeers = %q/%q, want never/stale", got[0].NodeID, got[1].NodeID)
	}
}
