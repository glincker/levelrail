package probe

import (
	"context"
	"net"
	"testing"
	"time"
)

func tcpListener(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
	return ln.Addr().String()
}

func TestTCPProbe(t *testing.T) {
	holdOpen := func(c net.Conn) { time.Sleep(2 * time.Second); _ = c.Close() }
	banner := func(c net.Conn) { _, _ = c.Write([]byte("220 hi\r\n")); holdOpen(c) }
	dropNow := func(c net.Conn) { _ = c.Close() }

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := closed.Addr().String()
	_ = closed.Close()

	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{name: "silent listener passes", addr: tcpListener(t, holdOpen)},
		{name: "banner listener passes", addr: tcpListener(t, banner)},
		{name: "accept then drop fails", addr: tcpListener(t, dropNow), wantErr: true},
		{name: "nothing listening fails", addr: closedAddr, wantErr: true},
	}
	p := New(nil, nil, Limits{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Check(context.Background(), Target{Addr: tt.addr}, Config{TCP: true, Timeout: time.Second})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTCPConfigValidate(t *testing.T) {
	if err := (Config{TCP: true}).Validate(); err != nil {
		t.Fatalf("tcp alone: %v", err)
	}
	if err := (Config{TCP: true, Path: "/x"}).Validate(); err == nil {
		t.Fatal("tcp with path must be rejected")
	}
}
