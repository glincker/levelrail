// Package gamestatus probes a game server's own published port: the
// Minecraft Java server list ping, with a plain TCP connect for other games.
package gamestatus

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// Kinds of answer a probe can give.
const (
	KindMinecraft = "minecraft"
	KindTCP       = "tcp"
)

// Protocol limits of the Java list ping, not tunables.
const (
	maxVarIntBytes = 5
	// MaxPacketBytes bounds one status packet; a favicon response is ~10 KB.
	MaxPacketBytes = 256 << 10
	maxHostBytes   = 255
	// handshakeProtocol is a legacy version number; servers answer a status
	// request with their own version whatever the client claims.
	handshakeProtocol = 47
	nextStateStatus   = 1
	packetHandshake   = 0x00
	packetStatus      = 0x00
	packetPing        = 0x01
)

var (
	// ErrNotLoopback is returned for any target but this host's loopback.
	ErrNotLoopback = errors.New("gamestatus: target must be a loopback address")
	ErrBadPacket   = errors.New("gamestatus: malformed status response")
)

// Status is one probe result.
type Status struct {
	Kind        string
	Online      bool
	Version     string
	Players     int
	MaxPlayers  int
	LatencyMS   int64
	Description string
}

// Dialer opens the connection; tests replace it.
type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

// Probe checks one loopback host:port. A Minecraft answer wins; a port that
// accepts a connection but does not speak the protocol is reported as TCP-open.
// Only loopback targets are allowed so a probe can never reach another host.
func Probe(ctx context.Context, dial Dialer, addr string, timeout time.Duration) (Status, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return Status{}, err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return Status{}, ErrNotLoopback
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return Status{}, fmt.Errorf("gamestatus: bad port %q", portStr)
	}
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	conn, err := dial(dctx, "tcp", addr)
	if err != nil {
		return Status{}, err
	}
	connectMS := time.Since(start).Milliseconds()
	_ = conn.Close()

	conn, err = dial(dctx, "tcp", addr)
	if err != nil {
		return Status{Kind: KindTCP, Online: true, LatencyMS: connectMS}, nil //nolint:nilerr // first connect proved the port is open
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	st, err := ReadMinecraftStatus(conn, host, uint16(port)) //nolint:gosec // range checked above
	if err != nil {
		return Status{Kind: KindTCP, Online: true, LatencyMS: connectMS}, nil //nolint:nilerr // open port that does not speak the list ping
	}
	return st, nil
}

// ReadMinecraftStatus runs the handshake, status request and ping over rw.
func ReadMinecraftStatus(rw io.ReadWriter, host string, port uint16) (Status, error) {
	if len(host) > maxHostBytes {
		return Status{}, ErrBadPacket
	}
	var hs bytes.Buffer
	putVarInt(&hs, packetHandshake)
	putVarInt(&hs, handshakeProtocol)
	putVarInt(&hs, int32(len(host)))
	hs.WriteString(host)
	_ = binary.Write(&hs, binary.BigEndian, port)
	putVarInt(&hs, nextStateStatus)
	if err := writePacket(rw, hs.Bytes()); err != nil {
		return Status{}, err
	}
	if err := writePacket(rw, []byte{packetStatus}); err != nil {
		return Status{}, err
	}
	br := bufio.NewReader(rw)
	asked := time.Now()
	payload, err := readPacket(br)
	if err != nil {
		return Status{}, err
	}
	rtt := time.Since(asked).Milliseconds()
	st, err := parseStatus(payload)
	if err != nil {
		return Status{}, err
	}
	st.LatencyMS = rtt
	// The ping packet is optional for our purposes; the status round trip stands in if it fails.
	var ping bytes.Buffer
	putVarInt(&ping, packetPing)
	_ = binary.Write(&ping, binary.BigEndian, int64(0))
	pinged := time.Now()
	if writePacket(rw, ping.Bytes()) == nil {
		if pong, perr := readPacket(br); perr == nil && len(pong) > 0 && pong[0] == packetPing {
			st.LatencyMS = time.Since(pinged).Milliseconds()
		}
	}
	return st, nil
}

type statusJSON struct {
	Version struct {
		Name string `json:"name"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
	} `json:"players"`
	Description json.RawMessage `json:"description"`
}

func parseStatus(payload []byte) (Status, error) {
	r := bytes.NewReader(payload)
	id, err := readVarInt(r)
	if err != nil || id != packetStatus {
		return Status{}, ErrBadPacket
	}
	n, err := readVarInt(r)
	if err != nil || n < 0 || int(n) > r.Len() {
		return Status{}, ErrBadPacket
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return Status{}, ErrBadPacket
	}
	var sj statusJSON
	if err := json.Unmarshal(body, &sj); err != nil {
		return Status{}, ErrBadPacket
	}
	return Status{
		Kind: KindMinecraft, Online: true, Version: sj.Version.Name,
		Players: sj.Players.Online, MaxPlayers: sj.Players.Max,
		Description: motdText(sj.Description),
	}, nil
}

// motdText reads a plain string or the text field of a chat object; anything
// richer is dropped rather than rendered.
func motdText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return obj.Text
	}
	return ""
}

func writePacket(w io.Writer, body []byte) error {
	var buf bytes.Buffer
	putVarInt(&buf, int32(len(body))) //nolint:gosec // our own packets are tiny
	buf.Write(body)
	_, err := w.Write(buf.Bytes())
	return err
}

// readPacket reads one length-prefixed packet, refusing lengths above MaxPacketBytes before allocating.
func readPacket(r *bufio.Reader) ([]byte, error) {
	n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if n < 1 || n > MaxPacketBytes {
		return nil, ErrBadPacket
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func putVarInt(b *bytes.Buffer, v int32) {
	u := uint32(v) //nolint:gosec // varints carry the two's complement bits
	for u >= 0x80 {
		b.WriteByte(byte(u) | 0x80)
		u >>= 7
	}
	b.WriteByte(byte(u))
}

func readVarInt(r io.ByteReader) (int32, error) {
	var out uint32
	for i := 0; i < maxVarIntBytes; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		out |= uint32(c&0x7f) << (7 * i)
		if c&0x80 == 0 {
			return int32(out), nil //nolint:gosec // two's complement by protocol
		}
	}
	return 0, ErrBadPacket
}
