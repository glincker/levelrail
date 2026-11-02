package gamestatus

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func framed(parts ...func(*bytes.Buffer)) []byte {
	var body bytes.Buffer
	for _, p := range parts {
		p(&body)
	}
	var out bytes.Buffer
	putVarInt(&out, int32(body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func statusPacket(js string) []byte {
	return framed(
		func(b *bytes.Buffer) { putVarInt(b, 0) },
		func(b *bytes.Buffer) { putVarInt(b, int32(len(js))); b.WriteString(js) },
	)
}

// serve answers one connection with canned bytes after reading what the client sends.
func serve(t *testing.T, reply []byte, hold time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				_, _ = readPacket(br)
				_, _ = readPacket(br)
				_, _ = c.Write(reply)
				time.Sleep(hold)
			}()
		}
	}()
	return ln.Addr().String()
}

func TestProbeValidResponse(t *testing.T) {
	js := `{"version":{"name":"1.21.11"},"players":{"online":2,"max":10},"description":{"text":"hi"}}`
	addr := serve(t, statusPacket(js), 0)
	st, err := Probe(context.Background(), nil, addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if st.Kind != KindMinecraft || st.Version != "1.21.11" || st.Players != 2 || st.MaxPlayers != 10 || st.Description != "hi" {
		t.Fatalf("unexpected status %+v", st)
	}
}

func TestProbeFallsBackToTCP(t *testing.T) {
	cases := map[string][]byte{
		"truncated":       statusPacket(`{"version":{"name":"x"}`)[:6],
		"garbage":         []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
		"bad json":        statusPacket(`not json`),
		"oversized":       append([]byte{0xff, 0xff, 0xff, 0x7f}, 'x'),
		"varint too long": {0x80, 0x80, 0x80, 0x80, 0x80, 0x01},
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			addr := serve(t, reply, 0)
			st, err := Probe(context.Background(), nil, addr, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if st.Kind != KindTCP || !st.Online {
				t.Fatalf("want tcp-open fallback, got %+v", st)
			}
		})
	}
}

func TestReadMinecraftStatusRejectsBadFraming(t *testing.T) {
	for name, reply := range map[string][]byte{
		"oversized length":     {0xff, 0xff, 0xff, 0x7f},
		"varint too long":      {0x80, 0x80, 0x80, 0x80, 0x80, 0x01},
		"string beyond packet": framed(func(b *bytes.Buffer) { putVarInt(b, 0) }, func(b *bytes.Buffer) { putVarInt(b, 9999) }),
		"zero length":          {0x00},
	} {
		t.Run(name, func(t *testing.T) {
			c, s := net.Pipe()
			go func() {
				defer func() { _ = s.Close() }()
				br := bufio.NewReader(s)
				_, _ = readPacket(br)
				_, _ = readPacket(br)
				_, _ = s.Write(reply)
			}()
			_ = c.SetDeadline(time.Now().Add(time.Second))
			if _, err := ReadMinecraftStatus(c, "127.0.0.1", 25565); err == nil {
				t.Fatal("want error")
			}
			_ = c.Close()
		})
	}
}

func TestProbeTimeoutOnSilentServer(t *testing.T) {
	addr := serve(t, nil, 3*time.Second)
	start := time.Now()
	st, err := Probe(context.Background(), nil, addr, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if st.Kind != KindTCP || time.Since(start) > 2*time.Second {
		t.Fatalf("want a quick tcp-open result, got %+v after %s", st, time.Since(start))
	}
}

func TestProbeRefusesNonLoopback(t *testing.T) {
	for _, addr := range []string{"10.0.0.5:25565", "example.com:25565", "8.8.8.8:25565"} {
		if _, err := Probe(context.Background(), nil, addr, time.Second); !errors.Is(err, ErrNotLoopback) {
			t.Fatalf("%s: want ErrNotLoopback, got %v", addr, err)
		}
	}
}

func TestProbeClosedPort(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	_ = ln.Close()
	if _, err := Probe(context.Background(), nil, addr, time.Second); err == nil {
		t.Fatal("want connect error")
	}
}
