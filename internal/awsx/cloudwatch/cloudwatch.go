// Package cloudwatch reads CloudWatch metrics (Query API 2010-08-01) and CloudWatch
// Logs (JSON 1.1). Metric reads are billed by AWS, so callers go through Cache.
package cloudwatch

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/awsx"
)

// Service names are checked against the botocore models for monitoring and logs.
var (
	Service     = awsx.Service{Name: "monitoring", Prefix: "monitoring", SigningName: "monitoring"}
	LogsService = awsx.Service{Name: "logs", Prefix: "logs", SigningName: "logs", TargetPrefix: "Logs_20140328"}
)

// ServiceName is the key used in the guard ServicesEnabled map.
const ServiceName = "cloudwatch"

const (
	version = "2010-08-01"
	// maxQueries is the GetMetricData limit per call.
	maxQueries = 500
	maxPages   = 10
	// DefaultWindow is the read window for one metric view.
	DefaultWindow = 3 * time.Hour
	// DefaultPeriod is the datapoint period in seconds for one metric view.
	DefaultPeriod = 300
)

// ErrBadQuery is returned for a query that cannot be sent.
var ErrBadQuery = errors.New("cloudwatch: invalid metric query")

// Dimension is one name and value of a metric identity.
type Dimension struct{ Name, Value string }

// Query is one metric to read.
type Query struct {
	ID, Namespace, Name, Stat string
	Dimensions                []Dimension
	Period                    int
}

// Point is one datapoint.
type Point struct {
	T time.Time
	V float64
}

// Series is the answer for one Query, oldest point first.
type Series struct {
	ID, Label, Status string
	Points            []Point
}

// Client is the lazy CloudWatch module; it makes no call until a method runs.
type Client struct{ aws *awsx.Client }

// New wraps a shared client.
func New(c *awsx.Client) *Client { return &Client{aws: c} }

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func setDims(v url.Values, prefix string, dims []Dimension) {
	for i, d := range dims {
		p := prefix + ".member." + strconv.Itoa(i+1)
		v.Set(p+".Name", d.Name)
		v.Set(p+".Value", d.Value)
	}
}

// GetMetricData reads many metrics in as few calls as possible (500 per call) and
// follows NextToken. Results come oldest first.
func (c *Client) GetMetricData(ctx context.Context, region string, qs []Query, start, end time.Time) ([]Series, error) {
	byID := map[string]*Series{}
	var order []string
	for lo := 0; lo < len(qs); lo += maxQueries {
		chunk := qs[lo:min(lo+maxQueries, len(qs))]
		v := url.Values{"StartTime": {start.UTC().Format(time.RFC3339)}, "EndTime": {end.UTC().Format(time.RFC3339)}, "ScanBy": {"TimestampAscending"}}
		for i, q := range chunk {
			if q.ID == "" || q.Namespace == "" || q.Name == "" || q.Period < 1 {
				return nil, ErrBadQuery
			}
			stat := q.Stat
			if stat == "" {
				stat = "Average"
			}
			p := "MetricDataQueries.member." + strconv.Itoa(i+1)
			v.Set(p+".Id", q.ID)
			v.Set(p+".MetricStat.Metric.Namespace", q.Namespace)
			v.Set(p+".MetricStat.Metric.MetricName", q.Name)
			setDims(v, p+".MetricStat.Metric.Dimensions", q.Dimensions)
			v.Set(p+".MetricStat.Period", strconv.Itoa(q.Period))
			v.Set(p+".MetricStat.Stat", stat)
			if _, seen := byID[q.ID]; !seen {
				byID[q.ID] = &Series{ID: q.ID}
				order = append(order, q.ID)
			}
		}
		token := ""
		for page := 0; page < maxPages; page++ {
			if token != "" {
				v.Set("NextToken", token)
			}
			var out struct {
				Results []struct {
					ID         string   `xml:"Id"`
					Label      string   `xml:"Label"`
					Status     string   `xml:"StatusCode"`
					Timestamps []string `xml:"Timestamps>member"`
					Values     []string `xml:"Values>member"`
				} `xml:"GetMetricDataResult>MetricDataResults>member"`
				NextToken string `xml:"GetMetricDataResult>NextToken"`
			}
			if err := c.aws.Query(ctx, Service, region, "GetMetricData", version, v, &out); err != nil {
				return nil, err
			}
			for _, r := range out.Results {
				s := byID[r.ID]
				if s == nil {
					continue
				}
				s.Label, s.Status = r.Label, r.Status
				for i, ts := range r.Timestamps {
					if i >= len(r.Values) {
						break
					}
					f, err := strconv.ParseFloat(strings.TrimSpace(r.Values[i]), 64)
					if err != nil {
						continue
					}
					s.Points = append(s.Points, Point{T: parseTime(ts), V: f})
				}
			}
			if out.NextToken == "" {
				break
			}
			token = out.NextToken
		}
	}
	res := make([]Series, 0, len(order))
	for _, id := range order {
		s := byID[id]
		sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].T.Before(s.Points[j].T) })
		res = append(res, *s)
	}
	return res, nil
}

// Metric is one metric identity from ListMetrics.
type Metric struct {
	Namespace, Name string
	Dimensions      []Dimension
}

// ListMetrics lists metrics, optionally filtered by namespace, name and one or more dimensions.
func (c *Client) ListMetrics(ctx context.Context, region, namespace, name string, dims []Dimension) ([]Metric, error) {
	var res []Metric
	token := ""
	for page := 0; page < maxPages; page++ {
		v := url.Values{}
		if namespace != "" {
			v.Set("Namespace", namespace)
		}
		if name != "" {
			v.Set("MetricName", name)
		}
		setDims(v, "Dimensions", dims)
		if token != "" {
			v.Set("NextToken", token)
		}
		var out struct {
			Metrics []struct {
				Namespace string `xml:"Namespace"`
				Name      string `xml:"MetricName"`
				Dims      []struct {
					Name  string `xml:"Name"`
					Value string `xml:"Value"`
				} `xml:"Dimensions>member"`
			} `xml:"ListMetricsResult>Metrics>member"`
			NextToken string `xml:"ListMetricsResult>NextToken"`
		}
		if err := c.aws.Query(ctx, Service, region, "ListMetrics", version, v, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Metrics {
			x := Metric{Namespace: m.Namespace, Name: m.Name}
			for _, d := range m.Dims {
				x.Dimensions = append(x.Dimensions, Dimension{Name: d.Name, Value: d.Value})
			}
			res = append(res, x)
		}
		if out.NextToken == "" {
			break
		}
		token = out.NextToken
	}
	return res, nil
}

// Cache holds metric reads for a short TTL. Concurrent callers with the same key
// share one provider call, because every metric read is billed.
type Cache struct {
	ttl time.Duration
	now func() time.Time
	mu  sync.Mutex
	m   map[string]cacheEntry
	fly map[string]*flight
}

type cacheEntry struct {
	val any
	at  time.Time
}

type flight struct {
	done chan struct{}
	val  any
	err  error
}

// NewCache builds a cache; a TTL of zero or less disables storing results.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, now: time.Now, m: map[string]cacheEntry{}, fly: map[string]*flight{}}
}

// Cached runs fn once per key per TTL and reports whether the answer came from the cache.
func Cached[T any](c *Cache, key string, fn func() (T, error)) (T, bool, error) {
	var zero T
	c.mu.Lock()
	if e, ok := c.m[key]; ok && c.now().Sub(e.at) < c.ttl {
		c.mu.Unlock()
		return e.val.(T), true, nil
	}
	if f, ok := c.fly[key]; ok {
		c.mu.Unlock()
		<-f.done
		if f.err != nil {
			return zero, true, f.err
		}
		return f.val.(T), true, nil
	}
	f := &flight{done: make(chan struct{})}
	c.fly[key] = f
	c.mu.Unlock()

	v, err := fn()

	c.mu.Lock()
	delete(c.fly, key)
	if err == nil && c.ttl > 0 {
		c.m[key] = cacheEntry{val: v, at: c.now()}
		for k, e := range c.m {
			if c.now().Sub(e.at) >= c.ttl {
				delete(c.m, k)
			}
		}
	}
	c.mu.Unlock()
	f.val, f.err = v, err
	close(f.done)
	return v, false, err
}

// LogGroup is one CloudWatch Logs group.
type LogGroup struct {
	Name          string
	RetentionDays int
	StoredBytes   int64
}

// DescribeLogGroups lists groups by name prefix, up to limit (all pages until then).
func (c *Client) DescribeLogGroups(ctx context.Context, region, prefix string, limit int) ([]LogGroup, error) {
	if limit < 1 {
		limit = 50
	}
	var res []LogGroup
	token := ""
	for page := 0; page < maxPages && len(res) < limit; page++ {
		in := map[string]any{"limit": min(limit-len(res), 50)}
		if prefix != "" {
			in["logGroupNamePrefix"] = prefix
		}
		if token != "" {
			in["nextToken"] = token
		}
		var out struct {
			Groups []struct {
				Name      string `json:"logGroupName"`
				Retention int    `json:"retentionInDays"`
				Stored    int64  `json:"storedBytes"`
			} `json:"logGroups"`
			Next string `json:"nextToken"`
		}
		if err := c.aws.JSON(ctx, LogsService, region, "DescribeLogGroups", in, &out); err != nil {
			return nil, err
		}
		for _, g := range out.Groups {
			res = append(res, LogGroup{Name: g.Name, RetentionDays: g.Retention, StoredBytes: g.Stored})
		}
		if out.Next == "" {
			break
		}
		token = out.Next
	}
	return res, nil
}

// FilterInput selects log events.
type FilterInput struct {
	Group, Pattern string
	Start, End     time.Time
	Limit          int
}

// Event is one log event.
type Event struct {
	Time            time.Time
	Stream, Message string
}

// FilterLogEvents reads matching events in the window, up to Limit. truncated
// reports that more events exist than were returned.
func (c *Client) FilterLogEvents(ctx context.Context, region string, f FilterInput) (events []Event, truncated bool, err error) {
	if f.Group == "" {
		return nil, false, ErrBadQuery
	}
	if f.Limit < 1 {
		f.Limit = 100
	}
	token := ""
	for page := 0; page < maxPages; page++ {
		in := map[string]any{"logGroupName": f.Group, "limit": min(f.Limit-len(events), 10000)}
		if f.Pattern != "" {
			in["filterPattern"] = f.Pattern
		}
		if !f.Start.IsZero() {
			in["startTime"] = f.Start.UnixMilli()
		}
		if !f.End.IsZero() {
			in["endTime"] = f.End.UnixMilli()
		}
		if token != "" {
			in["nextToken"] = token
		}
		var out struct {
			Events []struct {
				Stream  string `json:"logStreamName"`
				TS      int64  `json:"timestamp"`
				Message string `json:"message"`
			} `json:"events"`
			Next string `json:"nextToken"`
		}
		if err := c.aws.JSON(ctx, LogsService, region, "FilterLogEvents", in, &out); err != nil {
			return nil, false, err
		}
		for _, e := range out.Events {
			events = append(events, Event{Time: time.UnixMilli(e.TS).UTC(), Stream: e.Stream, Message: e.Message})
		}
		if out.Next == "" {
			return events, false, nil
		}
		if len(events) >= f.Limit {
			return events, true, nil
		}
		token = out.Next
	}
	return events, true, nil
}
