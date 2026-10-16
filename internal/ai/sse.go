package ai

import (
	"bufio"
	"io"
	"strings"
)

const sseMaxLine = 4 * 1024 * 1024

// readSSEData calls fn with the payload of every "data:" line until the
// stream ends or fn returns an error (including errStopSSE to stop early).
func readSSEData(r io.Reader, fn func(data string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), sseMaxLine)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok {
			continue
		}
		if err := fn(strings.TrimPrefix(data, " ")); err != nil {
			return err
		}
	}
	return sc.Err()
}
