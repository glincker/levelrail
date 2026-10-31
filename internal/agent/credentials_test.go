package agent

import (
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestNewServerCredentials_RejectsTLS12(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	creds, err := NewServerCredentials(ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("NewServerCredentials() error = %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		if sc, _, err := creds.ServerHandshake(c); err == nil {
			_ = sc.Close()
		}
	}()
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	cl := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12, NextProtos: []string{"h2"}}) //nolint:gosec // only the protocol version is under test
	if err := cl.Handshake(); err == nil {
		t.Fatal("TLS 1.2 handshake succeeded, want rejection")
	}
}
