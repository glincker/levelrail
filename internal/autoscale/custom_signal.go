package autoscale

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MetricGaugePrefix marks a policy metric that reads a gauge from the app's own metrics path, as "gauge:<name>".
const MetricGaugePrefix = "gauge:"

// Settings read by the gauge scraper.
const (
	EnvGaugePath    = "APP_AUTOSCALE_GAUGE_PATH"
	EnvGaugeTimeout = "APP_AUTOSCALE_GAUGE_TIMEOUT"
	EnvGaugeMaxBody = "APP_AUTOSCALE_GAUGE_MAX_BYTES"

	DefaultGaugePath    = "/metrics"
	DefaultGaugeTimeout = 3 * time.Second
	DefaultGaugeMaxBody = 1 << 20
)

var gaugeNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]{0,127}$`)

// GaugeName returns the gauge a policy reads, or "" when the metric is a built-in one.
func GaugeName(metric string) string {
	return strings.TrimPrefix(metric, MetricGaugePrefix)
}

// IsGaugeMetric reports whether the metric reads an app gauge.
func IsGaugeMetric(metric string) bool {
	return strings.HasPrefix(metric, MetricGaugePrefix)
}

func validGaugeMetric(metric string) error {
	if !gaugeNameRE.MatchString(GaugeName(metric)) {
		return errors.New("gauge metric must look like gauge:<name> with a plain metric name")
	}
	return nil
}

// GaugePathFromEnv is the path scraped on the app, always rooted.
func GaugePathFromEnv(lookup func(string) string) string {
	p := strings.TrimSpace(lookup(EnvGaugePath))
	if p == "" {
		return DefaultGaugePath
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// ParseGauge reads the first unlabeled or labeled sample of name from Prometheus text. A series with several label sets reports the largest value, the safe side for scale out.
func ParseGauge(r io.Reader, name string) (float64, bool) {
	sc := bufio.NewScanner(r)
	best, found := 0.0, false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || !strings.HasPrefix(line, name) {
			continue
		}
		rest := line[len(name):]
		if rest == "" || (rest[0] != ' ' && rest[0] != '{' && rest[0] != '\t') {
			continue
		}
		if rest[0] == '{' {
			end := strings.LastIndexByte(rest, '}')
			if end < 0 {
				continue
			}
			rest = rest[end+1:]
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if !found || v > best {
			best, found = v, true
		}
	}
	return best, found
}

// ScrapeGauge fetches path from base and returns the named gauge. base is a trusted container address, never caller input.
func ScrapeGauge(ctx context.Context, client *http.Client, base, path, name string, maxBody int64) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("scrape %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("scrape %s: status %d", path, resp.StatusCode)
	}
	v, ok := ParseGauge(io.LimitReader(resp.Body, maxBody), name)
	if !ok {
		return 0, fmt.Errorf("gauge %s not found at %s", name, path)
	}
	return v, nil
}
