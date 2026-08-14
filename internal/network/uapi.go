package network

// This file: translation between DeviceConfig/Status and WireGuard's UAPI
// text protocol.
//
// UAPI is the configuration interface every WireGuard implementation
// speaks. wireguard-go exposes it in-process as Device.IpcSet/IpcGet, and
// the in-kernel module exposes the same protocol over a unix socket that
// wg(8) itself uses. That is the reason this package can drive both
// backends from one encoder: the kernel path and the userspace path are
// not two config formats behind an abstraction, they are the same format
// reached two ways.
//
// Kept as pure string functions rather than methods on Device so the part
// that is easy to get subtly wrong (hex versus base64, seconds versus
// nanoseconds, which key resets which list) is testable without a device,
// root, or a kernel module. A wrong byte here is a peer that silently
// never handshakes.

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// EncodeUAPI renders cfg as a UAPI "set" payload, given the set of peer
// public keys the device currently has.
//
// current is what makes this a converging apply rather than a destructive
// one. The obvious encoding is to emit replace_peers=true and let the
// device discard everything it had, which is correct in the level-
// triggered sense and one line shorter. It is rejected here because
// wireguard-go's replace_peers destroys and rebuilds every Peer object,
// discarding the established session keys with them, so every reconcile
// tick would force a fresh handshake with every node in the fleet. On a
// 15-second resync that is a mesh that never finishes handshaking.
//
// Instead: peers in current but not in cfg are explicitly removed, and
// peers in cfg are upserted. An upsert of an unchanged peer leaves its
// session untouched, so a reconcile pass that changes nothing costs
// nothing, which is the property CLAUDE.md 4.2's level-triggered design
// depends on to be run repeatedly and safely.
//
// The private key is written first and only when set. A zero PrivateKey
// is skipped rather than written as 32 zero bytes: writing zeros would
// tell the device to discard its identity, which is the opposite of
// "this field is not being changed."
func EncodeUAPI(cfg DeviceConfig, current []Key) string {
	var b strings.Builder

	if !cfg.PrivateKey.IsZero() {
		fmt.Fprintf(&b, "private_key=%s\n", cfg.PrivateKey.Hex())
	}
	if cfg.ListenPort > 0 {
		fmt.Fprintf(&b, "listen_port=%d\n", cfg.ListenPort)
	}

	desired := make(map[Key]struct{}, len(cfg.Peers))
	for _, p := range cfg.Peers {
		desired[p.PublicKey] = struct{}{}
	}
	for _, k := range current {
		if _, keep := desired[k]; keep {
			continue
		}
		fmt.Fprintf(&b, "public_key=%s\nremove=true\n", k.Hex())
	}

	for _, p := range cfg.Peers {
		fmt.Fprintf(&b, "public_key=%s\n", p.PublicKey.Hex())
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
		}
		// Always written, including as 0, unlike private_key above: 0 is
		// a meaningful value here ("no keepalive"), not an unset one, and
		// omitting it would leave a previously-set interval in place on
		// an existing peer.
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", int(p.PersistentKeepalive.Seconds()))
		// replace_allowed_ips resets this peer's routes to exactly what
		// follows. Without it, allowed_ip lines accumulate across
		// applies and a peer that moved would keep routing its old
		// address forever.
		b.WriteString("replace_allowed_ips=true\n")
		for _, ip := range p.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", ip.String())
		}
	}

	// UAPI payloads are terminated by a blank line.
	b.WriteString("\n")
	return b.String()
}

// ParseUAPIStatus parses a UAPI "get" response into a Status.
//
// The peer entries in a UAPI response are positional, not nested: a
// public_key= line begins a new peer and every subsequent key belongs to
// it until the next public_key=. Getting that wrong attributes one peer's
// handshake time to another, which is why this is a real parser with a
// test rather than a regexp over the blob.
//
// nodeIDs maps public keys back to node IDs so the returned PeerStatus
// values can be logged by node ID (CLAUDE.md 7). WireGuard has no idea
// what a node ID is, so a peer whose key is not in the map gets an empty
// NodeID, which is itself informative: it is a peer on the device that no
// current plan accounts for.
func ParseUAPIStatus(raw string, nodeIDs map[Key]string) (Status, error) {
	st := Status{}
	var (
		peer          *PeerStatus
		handshakeSec  int64
		handshakeNsec int64
	)

	flushPeer := func() {
		if peer == nil {
			return
		}
		if handshakeSec != 0 || handshakeNsec != 0 {
			peer.LastHandshake = time.Unix(handshakeSec, handshakeNsec).UTC()
		}
		st.Peers = append(st.Peers, *peer)
		peer, handshakeSec, handshakeNsec = nil, 0, 0
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Status{}, fmt.Errorf("network: parse uapi status: malformed line %q", line)
		}

		switch key {
		case "private_key":
			// The device's own private key comes back in a get response.
			// It is read only to derive the public key (which is what
			// callers actually want in Status) and is never retained.
			k, err := parseHexKey(value)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: private_key: %w", err)
			}
			pub, err := k.PublicKey()
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: derive public key: %w", err)
			}
			st.PublicKey = pub

		case "listen_port":
			n, err := strconv.Atoi(value)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: listen_port %q: %w", value, err)
			}
			st.ListenPort = n

		case "public_key":
			flushPeer()
			k, err := parseHexKey(value)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: public_key: %w", err)
			}
			peer = &PeerStatus{PublicKey: k, NodeID: nodeIDs[k]}

		case "endpoint":
			if peer != nil {
				peer.Endpoint = value
			}

		case "last_handshake_time_sec":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: last_handshake_time_sec %q: %w", value, err)
			}
			handshakeSec = n

		case "last_handshake_time_nsec":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: last_handshake_time_nsec %q: %w", value, err)
			}
			handshakeNsec = n

		case "rx_bytes", "tx_bytes":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return Status{}, fmt.Errorf("network: parse uapi status: %s %q: %w", key, value, err)
			}
			if peer != nil {
				if key == "rx_bytes" {
					peer.TransferRx = n
				} else {
					peer.TransferTx = n
				}
			}

		case "errno":
			// The device reports its own result at the end of a get.
			// A non-zero errno means the response above it is not
			// trustworthy, so it is an error rather than a field to skip.
			if value != "0" {
				return Status{}, fmt.Errorf("network: uapi get returned errno=%s", value)
			}

		default:
			// allowed_ip, protocol_version, fwmark and anything a future
			// WireGuard adds: not needed for Status, and unknown keys
			// must not be an error or a WireGuard upgrade would break
			// status reporting.
		}
	}
	flushPeer()
	return st, nil
}

// parseHexKey decodes UAPI's lowercase-hex key encoding. Distinct from
// ParseKey (base64) because the two formats appear in different places
// and silently accepting either would hide a mistake at the boundary.
func parseHexKey(s string) (Key, error) {
	if len(s) != KeyLen*2 {
		return Key{}, fmt.Errorf("got %d hex chars, want %d", len(s), KeyLen*2)
	}
	var k Key
	for i := 0; i < KeyLen; i++ {
		hi, err := hexNibble(s[i*2])
		if err != nil {
			return Key{}, err
		}
		lo, err := hexNibble(s[i*2+1])
		if err != nil {
			return Key{}, err
		}
		k[i] = hi<<4 | lo
	}
	return k, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character %q", string(rune(c)))
	}
}

// peerKeys extracts the public keys from a Status's peers, the shape
// EncodeUAPI's current parameter wants. Apply's read-then-diff step is
// the only caller; it exists as a named function so that step reads as
// "what the device has now" rather than an inline loop.
func peerKeys(st Status) []Key {
	out := make([]Key, 0, len(st.Peers))
	for _, p := range st.Peers {
		out = append(out, p.PublicKey)
	}
	return out
}

// hostPrefix is the single-address prefix form used for a peer's own mesh
// address. Named rather than inlined because "a peer owns exactly its own
// address and nothing else" is a decision (see PeerConfig.AllowedIPs),
// not an implementation detail.
func hostPrefix(a netip.Addr) netip.Prefix {
	return netip.PrefixFrom(a, a.BitLen())
}
