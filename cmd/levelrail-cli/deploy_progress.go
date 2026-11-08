package main

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

const (
	envDeployProgressInterval     = "APP_CLI_PROGRESS_INTERVAL"
	defaultDeployProgressInterval = 15 * time.Second
)

// deployProgressInterval reads the quiet-period length after which a plain status line is printed.
func deployProgressInterval(lookupEnv func(string) (string, bool)) time.Duration {
	if v, ok := lookupEnv(envDeployProgressInterval); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			return d
		}
	}
	return defaultDeployProgressInterval
}

// startDeployProgress prints one plain line whenever nothing was printed for interval, so output piped
// through tail or a CI log never looks hung. Plain lines only: no spinner, no cursor control codes.
// touch records that a line was just printed; stop ends the ticker.
func startDeployProgress(w io.Writer, label string, interval time.Duration) (touch func(), stop func()) {
	start := time.Now()
	var last atomic.Int64
	last.Store(start.UnixNano())
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-t.C:
				if now.UnixNano()-last.Load() >= int64(interval) {
					_, _ = fmt.Fprintf(w, "  %sstill %s, %s elapsed\n", styleFor(w).symbol("… ", ""), label, now.Sub(start).Round(time.Second))
					last.Store(now.UnixNano())
				}
			}
		}
	}()
	return func() { last.Store(time.Now().UnixNano()) }, func() { close(done) }
}
