// Package metrics scrapes an app's own metrics endpoint and parses the text
// exposition format into capped, validated samples.
package metrics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// Limits bound one scrape; every field comes from runtime settings.
type Limits struct {
	Timeout      time.Duration
	MaxBodyBytes int64
	MaxSeries    int
	MaxLabels    int
	// MaxLabelBytes caps the canonical label text of one whole scrape.
	MaxLabelBytes int
}

// Sample is one parsed series value. Labels is the canonical sorted form.
type Sample struct {
	Name   string
	Labels string
	Value  float64
}

// Stats reports what a parse kept and dropped.
type Stats struct {
	Kept      int
	Dropped   int
	Truncated bool
}

// Identifier and value lengths are format rules of the exposition text, not tunables.
const (
	maxNameLen       = 200
	maxLabelKeyLen   = 64
	maxLabelValueLen = 256
	maxPathLen       = 200
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	pathRE       = regexp.MustCompile(`^/[A-Za-z0-9._~/-]*$`)
)

var (
	// ErrBodyTooLarge fails a scrape whose body is over the limit; nothing is stored.
	ErrBodyTooLarge = errors.New("metrics: response body exceeds the size limit")
	// ErrRedirect fails a scrape that answered with a redirect.
	ErrRedirect = errors.New("metrics: redirects are refused")
	// ErrBadTarget fails a target outside the app's private network.
	ErrBadTarget = errors.New("metrics: target is not an app network address")
)

// ValidName reports whether s is a valid metric name.
func ValidName(s string) bool { return len(s) <= maxNameLen && metricNameRE.MatchString(s) }

// ValidatePath accepts a plain absolute path with no query, host or traversal parts.
func ValidatePath(p string) error {
	if len(p) == 0 || len(p) > maxPathLen || !pathRE.MatchString(p) || strings.HasPrefix(p, "//") || strings.Contains(p, "..") {
		return fmt.Errorf("metrics path must be a plain absolute path like /metrics")
	}
	return nil
}

// ValidatePort accepts a TCP port number.
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("metrics port must be between 1 and 65535")
	}
	return nil
}

// TargetURL builds the scrape URL from the container's own network address only.
// A non-private address is refused so a bad lookup can never reach the outside.
func TargetURL(ip string, port int, path string) (string, error) {
	addr := net.ParseIP(ip)
	if addr == nil || !addr.IsPrivate() {
		return "", ErrBadTarget
	}
	if err := ValidatePort(port); err != nil {
		return "", err
	}
	if err := ValidatePath(path); err != nil {
		return "", err
	}
	return "http://" + net.JoinHostPort(addr.String(), strconv.Itoa(port)) + path, nil
}

// NewClient returns a direct HTTP client: no proxy, no redirects, a hard timeout.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: timeout}).DialContext, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirect
		},
	}
}

// Scrape fetches target and parses the body within lim.
func Scrape(ctx context.Context, client *http.Client, target string, lim Limits) ([]Sample, Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, Stats{}, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return nil, Stats{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, Stats{}, StatusError{Code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxBodyBytes+1))
	if err != nil {
		return nil, Stats{}, err
	}
	if int64(len(body)) > lim.MaxBodyBytes {
		return nil, Stats{}, ErrBodyTooLarge
	}
	samples, stats := ParseText(strings.NewReader(string(body)), lim)
	return samples, stats, nil
}

// ParseText parses exposition text, dropping invalid lines and stopping at MaxSeries.
func ParseText(r io.Reader, lim Limits) ([]Sample, Stats) {
	var (
		out        []Sample
		stats      Stats
		labelBytes int
		seen       = map[string]bool{}
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, ok := parseLine(line, lim.MaxLabels)
		if !ok {
			stats.Dropped++
			continue
		}
		key := s.Name + s.Labels
		if seen[key] {
			continue
		}
		if len(out) >= lim.MaxSeries || labelBytes+len(s.Labels) > lim.MaxLabelBytes {
			stats.Truncated = true
			stats.Dropped++
			continue
		}
		seen[key] = true
		labelBytes += len(s.Labels)
		out = append(out, s)
	}
	if sc.Err() != nil {
		stats.Truncated = true
	}
	stats.Kept = len(out)
	return out, stats
}

func parseLine(line string, maxLabels int) (Sample, bool) {
	end := strings.IndexAny(line, "{ \t")
	if end <= 0 {
		return Sample{}, false
	}
	name, rest := line[:end], line[end:]
	if !ValidName(name) {
		return Sample{}, false
	}
	labels := map[string]string{}
	if strings.HasPrefix(rest, "{") {
		var ok bool
		labels, rest, ok = parseLabels(rest[1:], maxLabels)
		if !ok {
			return Sample{}, false
		}
	}
	fields := strings.Fields(rest)
	if len(fields) < 1 || len(fields) > 2 {
		return Sample{}, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return Sample{}, false
	}
	return Sample{Name: name, Labels: canonicalLabels(labels), Value: v}, true
}

func parseLabels(s string, maxLabels int) (map[string]string, string, bool) {
	out := map[string]string{}
	for {
		s = strings.TrimLeft(s, " \t,")
		if strings.HasPrefix(s, "}") {
			return out, s[1:], true
		}
		eq := strings.IndexByte(s, '=')
		if eq <= 0 {
			return nil, "", false
		}
		key := strings.TrimSpace(s[:eq])
		if !labelNameRE.MatchString(key) || len(key) > maxLabelKeyLen {
			return nil, "", false
		}
		s = strings.TrimLeft(s[eq+1:], " \t")
		if !strings.HasPrefix(s, `"`) {
			return nil, "", false
		}
		val, rest, ok := readQuoted(s[1:])
		if !ok || len(val) > maxLabelValueLen || !utf8.ValidString(val) {
			return nil, "", false
		}
		out[key] = val
		if len(out) > maxLabels {
			return nil, "", false
		}
		s = rest
	}
}

func readQuoted(s string) (string, string, bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return b.String(), s[i+1:], true
		case '\\':
			if i+1 >= len(s) {
				return "", "", false
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", false
}

func canonicalLabels(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + strconv.Quote(m[k])
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// StatusError is a scrape that answered something other than 200.
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("metrics: endpoint answered %d", e.Code) }

// ErrorMessage maps a scrape failure to a fixed message, so stored and shown
// errors never carry the container address or raw transport text.
func ErrorMessage(err error) string {
	var se StatusError
	var ne net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrBodyTooLarge):
		return "response too large"
	case errors.Is(err, ErrRedirect):
		return "redirect refused"
	case errors.Is(err, ErrBadTarget):
		return "no address on the app network"
	case errors.As(err, &se):
		return fmt.Sprintf("endpoint answered status %d", se.Code)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timed out"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	default:
		return "scrape failed"
	}
}
