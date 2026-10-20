package probe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// tcpSettle is how long an accepted connection must stay open to count: a
// published-port proxy accepts and then drops when nothing listens behind it.
const tcpSettle = 250 * time.Millisecond

func (p *Prober) tcpAttempt(ctx context.Context, addr string, timeout time.Duration) (err error) {
	start := time.Now()
	defer func() { p.report(start, addr, 0, 0, err) }()
	desc := "TCP connect " + addr

	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(attemptCtx, "tcp", addr)
	if err != nil {
		kind := FailureConnect
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			kind = FailureTimeout
		}
		return &Failure{Kind: kind, Reason: fmt.Sprintf("%s failed: %v; nothing is listening on that port", desc, err), Err: err}
	}
	defer func() { _ = conn.Close() }()

	settle := tcpSettle
	if settle > timeout {
		settle = timeout
	}
	_ = conn.SetReadDeadline(time.Now().Add(settle))
	var one [1]byte
	n, rerr := conn.Read(one[:])
	if n > 0 || rerr == nil || errors.Is(rerr, os.ErrDeadlineExceeded) {
		return nil
	}
	return &Failure{Kind: FailureConnect, Reason: fmt.Sprintf("%s was accepted then closed (%v); the app is not listening on the declared port", desc, rerr), Err: rerr}
}
