package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	dockerclient "github.com/moby/moby/client"

	"github.com/moby/moby/api/pkg/stdcopy"
)

// LogLine is one demultiplexed, timestamped line of container log output.
type LogLine struct {
	// Stream is "stdout" or "stderr".
	Stream string
	// Timestamp is Docker's own recorded time for this line, parsed from
	// the log stream (Logs always requests Timestamps: true). Zero if the
	// line didn't parse as the expected "<timestamp> <message>" shape;
	// callers should fall back to their own clock in that case rather
	// than treating a parse failure as fatal.
	Timestamp time.Time
	Message   string
}

// LogReader is the optional capability of a runtime that can stream container logs.
type LogReader interface {
	Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan LogLine, <-chan error)
}

// Logs streams a container's log output, demultiplexed into a channel of
// individually stream-tagged, timestamped lines. Reads directly from the
// Docker Engine API's log endpoint, per the observability design rule.
func (c *Client) Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan LogLine, <-chan error) {
	out := make(chan LogLine)
	errCh := make(chan error, 1)

	opts := dockerclient.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
	}
	if !since.IsZero() {
		opts.Since = fmt.Sprintf("%d.%09d", since.Unix(), since.Nanosecond())
	}

	reader, err := c.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		errCh <- fmt.Errorf("docker: logs %s: %w", containerID, err)
		close(out)
		close(errCh)
		return out, errCh
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	// stdcopy.StdCopy blocks reading from reader (the multiplexed HTTP
	// response body) until the stream ends; its two writers are the pipe
	// write ends the two scan goroutines below read from, one per stream.
	go func() {
		defer func() { _ = reader.Close() }()
		_, copyErr := stdcopy.StdCopy(stdoutW, stderrW, reader)
		_ = stdoutW.CloseWithError(copyErr)
		_ = stderrW.CloseWithError(copyErr)
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	scan := func(stream string, r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow log lines up to 1MB
		for scanner.Scan() {
			ts, message := parseLogLine(scanner.Text())
			select {
			case out <- LogLine{Stream: stream, Timestamp: ts, Message: message}:
			case <-ctx.Done():
				return
			}
		}
		// A cancelled ctx tears the pipe down from the copy goroutine
		// above, which surfaces here as a read error too; that's a clean
		// shutdown, not a failure worth reporting, so it's the only case
		// deliberately not sent to errCh.
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case errCh <- fmt.Errorf("docker: scan %s logs for %s: %w", stream, containerID, err):
			default: // the other stream's scan goroutine already reported one
			}
		}
	}
	go scan("stdout", stdoutR)
	go scan("stderr", stderrR)

	go func() {
		wg.Wait()
		close(out)
		close(errCh)
	}()

	return out, errCh
}

// parseLogLine splits one line of Docker's log output requested with
// Timestamps: true into its RFC3339Nano timestamp prefix and the message
// that follows it. Docker's own format is "<timestamp> <message>" with
// exactly one space as the separator between the two; a line that
// doesn't parse as that shape (or whose prefix isn't a valid timestamp)
// is returned as-is with a zero Timestamp, for the caller to fall back
// to its own clock rather than treating a parse failure as fatal.
func parseLogLine(raw string) (ts time.Time, message string) {
	idx := strings.IndexByte(raw, ' ')
	if idx < 0 {
		return time.Time{}, raw
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw[:idx])
	if err != nil {
		return time.Time{}, raw
	}
	return parsed, raw[idx+1:]
}
