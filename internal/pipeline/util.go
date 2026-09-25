package pipeline

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func baseJobName(key string) string {
	if i := strings.IndexByte(key, '['); i >= 0 {
		return key[:i]
	}
	return key
}

func parseMemory(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"K", 1000}, {"M", 1000 * 1000}, {"G", 1000 * 1000 * 1000}} {
		if strings.HasSuffix(s, u.suffix) {
			mult = u.mult
			s = strings.TrimSuffix(s, u.suffix)
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q", s)
	}
	return n * mult, nil
}

const maxLineBytes = 64 * 1024

// readLines calls fn for each line of r, truncating lines longer than
// maxLineBytes rather than failing the step on a pathological one.
func readLines(r io.Reader, fn func(string)) error {
	br := bufio.NewReaderSize(r, 32*1024)
	var cur []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if len(cur) < maxLineBytes {
			cur = append(cur, chunk...)
		}
		if err != nil {
			if len(cur) > 0 {
				fn(string(cur))
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
		if !isPrefix {
			fn(string(cur))
			cur = cur[:0]
		}
	}
}

// masker replaces secret values in output with ***.
type masker struct {
	mu   sync.Mutex
	vals []string
}

func (m *masker) add(v string) {
	if len(v) < 3 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vals = append(m.vals, v)
}

func (m *masker) mask(s string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.vals {
		s = strings.ReplaceAll(s, v, "***")
	}
	return s
}
