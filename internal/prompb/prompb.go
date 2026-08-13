// Package prompb implements the small subset of Prometheus's remote-read
// wire protocol TASKS.md 2.6 needs (ReadRequest/ReadResponse and their
// nested messages), by hand, using google.golang.org/protobuf's
// low-level protowire primitives (already a transitive dependency, no
// new module).
//
// Deliberately not github.com/prometheus/prometheus/prompb: that's the
// full Prometheus server module, known in practice to drag in a much
// heavier dependency tree than the handful of stable, simple messages
// this package actually needs, the same reasoning ADR 009 already
// applied against embedding VictoriaMetrics's storage engine for the
// metrics store itself. The remote-read wire format (field numbers,
// message shapes) has been stable for years across the Prometheus/
// Thanos/Cortex/Grafana Mimir ecosystem, precisely because changing it
// would break every tool that speaks it; hand-encoding against that
// stable, public contract is a reasonable trade for staying dependency-
// light, unlike hand-rolling an internal format Levelrail alone reads.
package prompb

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// MatcherType mirrors prometheus/prometheus/prompb's LabelMatcher_Type
// enum values exactly; these are wire-format constants, not this
// package's own invention.
type MatcherType int32

// The four matcher types Prometheus's remote-read protocol defines.
// This pass's HTTP handler (internal/api/prometheus.go) only actually
// resolves MatchEqual; the others are still decoded correctly (an
// unsupported matcher type is a valid, representable value, not a
// decode error) so the codec itself stays a complete, honest
// implementation of the wire format even where the handler built on
// top of it is narrower.
const (
	MatchEqual     MatcherType = 0
	MatchNotEqual  MatcherType = 1
	MatchRegexp    MatcherType = 2
	MatchNotRegexp MatcherType = 3
)

// Label is one name/value pair identifying a time series, e.g.
// {__name__="levelrail_cpu_percent"} or {service="web"}.
type Label struct {
	Name  string
	Value string
}

// Sample is one metric value at one point in time. Timestamp is
// milliseconds since the Unix epoch, matching Prometheus's own
// convention (not seconds, and not this repo's more common time.Time).
type Sample struct {
	Value     float64
	Timestamp int64
}

// LabelMatcher is one selector term in a Query, e.g. Type=MatchEqual,
// Name="service", Value="web".
type LabelMatcher struct {
	Type  MatcherType
	Name  string
	Value string
}

// Query is one time-series selection: a time range plus the matchers
// that narrow which series it covers.
type Query struct {
	StartTimestampMs int64
	EndTimestampMs   int64
	Matchers         []LabelMatcher
}

// ReadRequest is the top-level message a remote-read client (Prometheus
// or Grafana pointed directly at this control plane) sends.
type ReadRequest struct {
	Queries []Query
}

// TimeSeries is one selected series: the labels that identify it plus
// every sample found for it within the query's time range.
type TimeSeries struct {
	Labels  []Label
	Samples []Sample
}

// QueryResult answers one Query with every matching TimeSeries.
type QueryResult struct {
	TimeSeries []TimeSeries
}

// ReadResponse is the top-level message this control plane sends back,
// one QueryResult per Query in the originating ReadRequest, same order.
type ReadResponse struct {
	Results []QueryResult
}

// Field numbers below are prompb's own stable wire contract (remote.proto
// and types.proto), not chosen by this package.
const (
	fieldReadRequestQueries = 1

	fieldQueryStart    = 1
	fieldQueryEnd      = 2
	fieldQueryMatchers = 3

	fieldMatcherType  = 1
	fieldMatcherName  = 2
	fieldMatcherValue = 3

	fieldReadResponseResults = 1

	fieldQueryResultTimeSeries = 1

	fieldTimeSeriesLabels  = 1
	fieldTimeSeriesSamples = 2

	fieldLabelName  = 1
	fieldLabelValue = 2

	fieldSampleValue     = 1
	fieldSampleTimestamp = 2
)

func appendEmbedded(b []byte, num protowire.Number, sub []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, sub)
}

func marshalLabel(l Label) []byte {
	var b []byte
	b = protowire.AppendTag(b, fieldLabelName, protowire.BytesType)
	b = protowire.AppendString(b, l.Name)
	b = protowire.AppendTag(b, fieldLabelValue, protowire.BytesType)
	b = protowire.AppendString(b, l.Value)
	return b
}

func unmarshalLabel(b []byte) (Label, error) {
	var l Label
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return Label{}, fmt.Errorf("prompb: Label: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldLabelName:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return Label{}, fmt.Errorf("prompb: Label.name: %w", protowire.ParseError(n))
			}
			l.Name = v
			b = b[n:]
		case fieldLabelValue:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return Label{}, fmt.Errorf("prompb: Label.value: %w", protowire.ParseError(n))
			}
			l.Value = v
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return Label{}, fmt.Errorf("prompb: Label: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return l, nil
}

func marshalSample(s Sample) []byte {
	var b []byte
	b = protowire.AppendTag(b, fieldSampleValue, protowire.Fixed64Type)
	b = protowire.AppendFixed64(b, math.Float64bits(s.Value))
	b = protowire.AppendTag(b, fieldSampleTimestamp, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(s.Timestamp)) //nolint:gosec // proto3 int64 varints are the raw two's complement bit pattern, not zigzag; this cast preserves it exactly, it never truncates
	return b
}

func unmarshalSample(b []byte) (Sample, error) {
	var s Sample
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return Sample{}, fmt.Errorf("prompb: Sample: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldSampleValue:
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return Sample{}, fmt.Errorf("prompb: Sample.value: %w", protowire.ParseError(n))
			}
			s.Value = math.Float64frombits(v)
			b = b[n:]
		case fieldSampleTimestamp:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return Sample{}, fmt.Errorf("prompb: Sample.timestamp: %w", protowire.ParseError(n))
			}
			s.Timestamp = int64(v) //nolint:gosec // reverses the exact bit-preserving cast in marshalSample above, not a truncation
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return Sample{}, fmt.Errorf("prompb: Sample: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return s, nil
}

func marshalMatcher(m LabelMatcher) []byte {
	var b []byte
	b = protowire.AppendTag(b, fieldMatcherType, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(m.Type)) //nolint:gosec // MatcherType is one of 4 small enum values (0-3), never near int32/uint64 range limits
	b = protowire.AppendTag(b, fieldMatcherName, protowire.BytesType)
	b = protowire.AppendString(b, m.Name)
	b = protowire.AppendTag(b, fieldMatcherValue, protowire.BytesType)
	b = protowire.AppendString(b, m.Value)
	return b
}

func unmarshalMatcher(b []byte) (LabelMatcher, error) {
	var m LabelMatcher
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return LabelMatcher{}, fmt.Errorf("prompb: LabelMatcher: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldMatcherType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return LabelMatcher{}, fmt.Errorf("prompb: LabelMatcher.type: %w", protowire.ParseError(n))
			}
			m.Type = MatcherType(v) //nolint:gosec // reverses the enum cast in marshalMatcher above; an out-of-range wire value just becomes an unrecognized MatcherType, not a crash
			b = b[n:]
		case fieldMatcherName:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return LabelMatcher{}, fmt.Errorf("prompb: LabelMatcher.name: %w", protowire.ParseError(n))
			}
			m.Name = v
			b = b[n:]
		case fieldMatcherValue:
			v, n := protowire.ConsumeString(b)
			if n < 0 {
				return LabelMatcher{}, fmt.Errorf("prompb: LabelMatcher.value: %w", protowire.ParseError(n))
			}
			m.Value = v
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return LabelMatcher{}, fmt.Errorf("prompb: LabelMatcher: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return m, nil
}

func marshalQuery(q Query) []byte {
	var b []byte
	b = protowire.AppendTag(b, fieldQueryStart, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(q.StartTimestampMs)) //nolint:gosec // exact bit-preserving cast, same reasoning as marshalSample's timestamp
	b = protowire.AppendTag(b, fieldQueryEnd, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(q.EndTimestampMs)) //nolint:gosec // exact bit-preserving cast, same reasoning as marshalSample's timestamp
	for _, m := range q.Matchers {
		b = appendEmbedded(b, fieldQueryMatchers, marshalMatcher(m))
	}
	return b
}

func unmarshalQuery(b []byte) (Query, error) {
	var q Query
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return Query{}, fmt.Errorf("prompb: Query: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldQueryStart:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return Query{}, fmt.Errorf("prompb: Query.start_timestamp_ms: %w", protowire.ParseError(n))
			}
			q.StartTimestampMs = int64(v) //nolint:gosec // reverses the exact bit-preserving cast in marshalQuery above, not a truncation
			b = b[n:]
		case fieldQueryEnd:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return Query{}, fmt.Errorf("prompb: Query.end_timestamp_ms: %w", protowire.ParseError(n))
			}
			q.EndTimestampMs = int64(v) //nolint:gosec // reverses the exact bit-preserving cast in marshalQuery above, not a truncation
			b = b[n:]
		case fieldQueryMatchers:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return Query{}, fmt.Errorf("prompb: Query.matchers: %w", protowire.ParseError(n))
			}
			m, err := unmarshalMatcher(sub)
			if err != nil {
				return Query{}, err
			}
			q.Matchers = append(q.Matchers, m)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return Query{}, fmt.Errorf("prompb: Query: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return q, nil
}

func marshalTimeSeries(ts TimeSeries) []byte {
	var b []byte
	for _, l := range ts.Labels {
		b = appendEmbedded(b, fieldTimeSeriesLabels, marshalLabel(l))
	}
	for _, s := range ts.Samples {
		b = appendEmbedded(b, fieldTimeSeriesSamples, marshalSample(s))
	}
	return b
}

func unmarshalTimeSeries(b []byte) (TimeSeries, error) {
	var ts TimeSeries
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return TimeSeries{}, fmt.Errorf("prompb: TimeSeries: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldTimeSeriesLabels:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return TimeSeries{}, fmt.Errorf("prompb: TimeSeries.labels: %w", protowire.ParseError(n))
			}
			l, err := unmarshalLabel(sub)
			if err != nil {
				return TimeSeries{}, err
			}
			ts.Labels = append(ts.Labels, l)
			b = b[n:]
		case fieldTimeSeriesSamples:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return TimeSeries{}, fmt.Errorf("prompb: TimeSeries.samples: %w", protowire.ParseError(n))
			}
			s, err := unmarshalSample(sub)
			if err != nil {
				return TimeSeries{}, err
			}
			ts.Samples = append(ts.Samples, s)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return TimeSeries{}, fmt.Errorf("prompb: TimeSeries: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return ts, nil
}

func marshalQueryResult(qr QueryResult) []byte {
	var b []byte
	for _, ts := range qr.TimeSeries {
		b = appendEmbedded(b, fieldQueryResultTimeSeries, marshalTimeSeries(ts))
	}
	return b
}

func unmarshalQueryResult(b []byte) (QueryResult, error) {
	var qr QueryResult
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return QueryResult{}, fmt.Errorf("prompb: QueryResult: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldQueryResultTimeSeries:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return QueryResult{}, fmt.Errorf("prompb: QueryResult.timeseries: %w", protowire.ParseError(n))
			}
			ts, err := unmarshalTimeSeries(sub)
			if err != nil {
				return QueryResult{}, err
			}
			qr.TimeSeries = append(qr.TimeSeries, ts)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return QueryResult{}, fmt.Errorf("prompb: QueryResult: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return qr, nil
}

// MarshalReadRequest encodes req as plain (uncompressed) protobuf bytes.
// The remote-read HTTP transport wraps this in snappy block compression;
// that's the HTTP handler's concern, not this package's, which only
// speaks the message format.
func MarshalReadRequest(req ReadRequest) []byte {
	var b []byte
	for _, q := range req.Queries {
		b = appendEmbedded(b, fieldReadRequestQueries, marshalQuery(q))
	}
	return b
}

// UnmarshalReadRequest decodes plain (uncompressed) protobuf bytes.
func UnmarshalReadRequest(b []byte) (ReadRequest, error) {
	var req ReadRequest
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return ReadRequest{}, fmt.Errorf("prompb: ReadRequest: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldReadRequestQueries:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return ReadRequest{}, fmt.Errorf("prompb: ReadRequest.queries: %w", protowire.ParseError(n))
			}
			q, err := unmarshalQuery(sub)
			if err != nil {
				return ReadRequest{}, err
			}
			req.Queries = append(req.Queries, q)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return ReadRequest{}, fmt.Errorf("prompb: ReadRequest: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return req, nil
}

// MarshalReadResponse encodes resp as plain (uncompressed) protobuf bytes.
func MarshalReadResponse(resp ReadResponse) []byte {
	var b []byte
	for _, r := range resp.Results {
		b = appendEmbedded(b, fieldReadResponseResults, marshalQueryResult(r))
	}
	return b
}

// UnmarshalReadResponse decodes plain (uncompressed) protobuf bytes.
// Exported for tests (round-tripping this package's own output) and for
// any future client-side use; the HTTP handler itself only ever calls
// MarshalReadResponse.
func UnmarshalReadResponse(b []byte) (ReadResponse, error) {
	var resp ReadResponse
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return ReadResponse{}, fmt.Errorf("prompb: ReadResponse: bad tag: %w", protowire.ParseError(n))
		}
		b = b[n:]
		switch num {
		case fieldReadResponseResults:
			sub, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return ReadResponse{}, fmt.Errorf("prompb: ReadResponse.results: %w", protowire.ParseError(n))
			}
			r, err := unmarshalQueryResult(sub)
			if err != nil {
				return ReadResponse{}, err
			}
			resp.Results = append(resp.Results, r)
			b = b[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return ReadResponse{}, fmt.Errorf("prompb: ReadResponse: skip unknown field %d: %w", num, protowire.ParseError(n))
			}
			b = b[n:]
		}
	}
	return resp, nil
}
