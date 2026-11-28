package application

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/docker"
)

// maxProcNetBytes caps one socket table read from a container.
const maxProcNetBytes = 256 << 10

// portMismatchError ends a readiness wait early: the app answers on a port
// other than the configured one, so waiting out the budget cannot help.
type portMismatchError struct {
	configured, actual int
}

func (e *portMismatchError) Error() string {
	return fmt.Sprintf("the app listens on port %d but the configured port is %d: change the app port to %d and redeploy", e.actual, e.configured, e.actual)
}

// containerListeningPorts reads the container's TCP listeners; ok is false
// when no socket table could be read (an image without cat, for one).
func containerListeningPorts(ctx context.Context, rt docker.Runtime, id string) ([]int, bool) {
	var tables []string
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		out, err := rt.Exec(ctx, id, []string{"cat", path})
		if err != nil || out == nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(out, maxProcNetBytes))
		_ = out.Close()
		tables = append(tables, string(b))
	}
	if len(tables) == 0 {
		return nil, false
	}
	return parseListeningPorts(tables), true
}

// parseListeningPorts lists the ports in LISTEN state (0A) in /proc/net/tcp
// tables, kept local so the reconciler does not depend on the diagnose package.
func parseListeningPorts(tables []string) []int {
	seen := map[int]bool{}
	var ports []int
	for _, table := range tables {
		for _, ln := range strings.Split(table, "\n") {
			f := strings.Fields(ln)
			if len(f) < 4 || f[3] != "0A" {
				continue
			}
			_, hexPort, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			p, err := strconv.ParseInt(hexPort, 16, 32)
			if err != nil || p <= 0 || seen[int(p)] {
				continue
			}
			seen[int(p)] = true
			ports = append(ports, int(p))
		}
	}
	return ports
}

// suggestPort names the real port when the app listens on exactly one port and
// it is not the configured one; several listeners are ambiguous, so no guess.
func suggestPort(configured int, listening []int) (int, bool) {
	if configured <= 0 || len(listening) != 1 || listening[0] == configured {
		return 0, false
	}
	return listening[0], true
}

// watchPortMismatch polls the container's listeners while the readiness probe
// fails. The same single wrong port seen on two polls in a row ends the wait,
// so one transient listener never fails a deploy.
func watchPortMismatch(ctx context.Context, rt docker.Runtime, id string, configured int, interval time.Duration, out chan<- error) {
	if configured <= 0 || interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	last := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ports, ok := containerListeningPorts(ctx, rt, id)
			actual, wrong := 0, false
			if ok {
				actual, wrong = suggestPort(configured, ports)
			}
			if wrong && actual == last {
				select {
				case out <- &portMismatchError{configured: configured, actual: actual}:
				default:
				}
				return
			}
			last = 0
			if wrong {
				last = actual
			}
		}
	}
}
