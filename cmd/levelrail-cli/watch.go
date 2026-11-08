package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

const (
	watchTimeoutEnv      = "LEVELRAIL_WATCH_TIMEOUT"
	watchPollEnv         = "LEVELRAIL_WATCH_POLL"
	watchTimeoutFallback = 30 * time.Minute
	watchPollFallback    = 2 * time.Second
	// A transient poll error is retried this many times in a row before the watch gives up.
	watchMaxPollErrors = 5
)

// watchOptions are the flags shared by every long operation that can be followed to the end.
type watchOptions struct {
	wait    bool
	watch   bool
	timeout time.Duration
}

func (w *watchOptions) register(fs *flag.FlagSet) {
	fs.BoolVar(&w.wait, "wait", false, "block until the operation finishes; exit 0 on success, 1 on failure")
	w.registerWatch(fs)
}

// registerWatch adds only --watch, for commands whose --wait already means something else.
func (w *watchOptions) registerWatch(fs *flag.FlagSet) {
	fs.BoolVar(&w.watch, "watch", false, "like --wait, with a live progress line on stderr")
	fs.DurationVar(&w.timeout, "watch-timeout", 0, "with --wait or --watch, give up after this long (default "+watchTimeoutEnv+", then 30m)")
}

func (w watchOptions) enabled() bool { return w.wait || w.watch }

// watchState is one poll of an operation row.
type watchState struct {
	Status string
	Phase  string
	Error  string
}

// terminal reports whether the operation stopped, and whether it ended well.
func (s watchState) terminal() (done, ok bool) {
	switch s.Status {
	case "succeeded", "completed", "done":
		return true, true
	case "failed", "error", "rolled_back", "canceled", "cancelled":
		return true, false
	}
	return false, false
}

// followOperation polls fetch until the operation ends. Progress goes to stderr only, so
// stdout stays machine readable. It returns exitOK on success, 1 on failure.
func followOperation(ctx context.Context, w watchOptions, label string, stderr io.Writer, lookupEnv func(string) (string, bool), fetch func(context.Context) (watchState, error)) int {
	timeout := w.timeout
	if timeout <= 0 {
		timeout = durationFromEnv(lookupEnv, watchTimeoutEnv, watchTimeoutFallback)
	}
	interval := durationFromEnv(lookupEnv, watchPollEnv, watchPollFallback)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	live := w.watch && exportStdoutIsTerminal(stderr)
	last := ""
	failures := 0
	for {
		st, err := fetch(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			failures++
			if failures >= watchMaxPollErrors {
				_, _ = fmt.Fprintf(stderr, "\n%s: lost track of %s: %v\n", "watch", label, err)
				return exitNetwork
			}
		case err == nil:
			failures = 0
			line := progressLine(label, st)
			if w.watch && line != last {
				printProgress(stderr, line, live)
				last = line
			}
			if done, ok := st.terminal(); done {
				if live {
					_, _ = fmt.Fprintln(stderr)
				}
				if ok {
					_, _ = fmt.Fprintf(stderr, "%s finished: %s\n", label, st.Status)
					return exitOK
				}
				_, _ = fmt.Fprintf(stderr, "%s failed: %s\n", label, failureText(st))
				return exitCheckFailed
			}
		}
		select {
		case <-ctx.Done():
			if live {
				_, _ = fmt.Fprintln(stderr)
			}
			_, _ = fmt.Fprintf(stderr, "%s still running after %s; it continues on the server\n", label, timeout)
			return exitDeployTimeout
		case <-time.After(interval):
		}
	}
}

func progressLine(label string, st watchState) string {
	line := label + ": " + st.Status
	if st.Phase != "" {
		line += " (" + st.Phase + ")"
	}
	return line
}

func failureText(st watchState) string {
	if st.Error != "" {
		return st.Error
	}
	return st.Status
}

func printProgress(w io.Writer, line string, live bool) {
	if live {
		_, _ = fmt.Fprintf(w, "\r\x1b[2K%s", line)
		return
	}
	_, _ = fmt.Fprintln(w, line)
}

const watchUsageFlags = `  --wait                   block until the operation finishes; exit 0 on success, 1 on failure
  --watch                  like --wait, with a live progress line on stderr
  --watch-timeout duration give up after this long (default: ` + watchTimeoutEnv + ` env var, then 30m)
`

const watchOnlyUsageFlags = `  --watch                  follow the clone to the end with a live progress line on stderr; exit 0 on success, 1 on failure
  --watch-timeout duration give up after this long (default: ` + watchTimeoutEnv + ` env var, then 30m)
`
