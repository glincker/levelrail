package prompb

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

// TestMarshalLabel_ExactBytes hand-verifies the wire encoding against
// protobuf's documented format directly, not just this package's own
// round-trip: a systematic bug present in both marshalLabel and
// unmarshalLabel could round-trip "successfully" while still being
// wire-incompatible with a real Prometheus/Grafana client, which is
// exactly the failure mode a round-trip-only test can't catch.
//
// Label{Name: "a", Value: "b"}: field 1 (name, wire type 2/bytes) tag
// = (1<<3)|2 = 0x0A, length 1, "a" (0x61); field 2 (value, same wire
// type) tag = (2<<3)|2 = 0x12, length 1, "b" (0x62).
func TestMarshalLabel_ExactBytes(t *testing.T) {
	got := marshalLabel(Label{Name: "a", Value: "b"})
	want := []byte{0x0A, 0x01, 0x61, 0x12, 0x01, 0x62}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("marshalLabel() = %#v, want %#v", got, want)
	}
}

// TestMarshalSample_ExactBytes: Sample{Value: 1.0, Timestamp: 1000}.
// Field 1 (value, wire type 1/fixed64) tag = (1<<3)|1 = 0x09, then the
// IEEE 754 bits of 1.0 (0x3FF0000000000000) little-endian. Field 2
// (timestamp, wire type 0/varint) tag = (2<<3)|0 = 0x10, then
// varint(1000) = [0xE8, 0x07] (1000 = 0b1111101000: low 7 bits 1101000
// with the continuation bit set = 0xE8, remaining bits 0000111 = 0x07).
func TestMarshalSample_ExactBytes(t *testing.T) {
	got := marshalSample(Sample{Value: 1.0, Timestamp: 1000})
	want := []byte{
		0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xF0, 0x3F,
		0x10, 0xE8, 0x07,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("marshalSample() = %#v, want %#v", got, want)
	}
}

func TestLabel_RoundTrip(t *testing.T) {
	want := Label{Name: "__name__", Value: "levelrail_cpu_percent"}
	got, err := unmarshalLabel(marshalLabel(want))
	if err != nil {
		t.Fatalf("unmarshalLabel() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestSample_RoundTrip(t *testing.T) {
	want := Sample{Value: 42.5, Timestamp: 1_700_000_000_000}
	got, err := unmarshalSample(marshalSample(want))
	if err != nil {
		t.Fatalf("unmarshalSample() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestSample_RoundTrip_NegativeAndZeroValues(t *testing.T) {
	tests := []Sample{
		{Value: 0, Timestamp: 0},
		{Value: -1.5, Timestamp: -1000}, // Prometheus allows timestamps before the epoch in principle; the codec must not assume non-negative
	}
	for _, want := range tests {
		got, err := unmarshalSample(marshalSample(want))
		if err != nil {
			t.Fatalf("unmarshalSample() error = %v", err)
		}
		if got != want {
			t.Errorf("round trip = %+v, want %+v", got, want)
		}
	}
}

func TestMatcher_RoundTrip(t *testing.T) {
	want := LabelMatcher{Type: MatchRegexp, Name: "service", Value: "web.*"}
	got, err := unmarshalMatcher(marshalMatcher(want))
	if err != nil {
		t.Fatalf("unmarshalMatcher() error = %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestQuery_RoundTrip(t *testing.T) {
	want := Query{
		StartTimestampMs: 1_700_000_000_000,
		EndTimestampMs:   1_700_003_600_000,
		Matchers: []LabelMatcher{
			{Type: MatchEqual, Name: "__name__", Value: "levelrail_cpu_percent"},
			{Type: MatchEqual, Name: "service", Value: "web"},
		},
	}
	got, err := unmarshalQuery(marshalQuery(want))
	if err != nil {
		t.Fatalf("unmarshalQuery() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestTimeSeries_RoundTrip(t *testing.T) {
	want := TimeSeries{
		Labels: []Label{
			{Name: "__name__", Value: "levelrail_cpu_percent"},
			{Name: "service", Value: "web"},
		},
		Samples: []Sample{
			{Value: 10, Timestamp: 1000},
			{Value: 20, Timestamp: 2000},
		},
	}
	got, err := unmarshalTimeSeries(marshalTimeSeries(want))
	if err != nil {
		t.Fatalf("unmarshalTimeSeries() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestReadRequest_RoundTrip(t *testing.T) {
	want := ReadRequest{
		Queries: []Query{
			{
				StartTimestampMs: 1000,
				EndTimestampMs:   2000,
				Matchers: []LabelMatcher{
					{Type: MatchEqual, Name: "__name__", Value: "levelrail_memory_usage_bytes"},
				},
			},
			{
				StartTimestampMs: 3000,
				EndTimestampMs:   4000,
			},
		},
	}
	got, err := UnmarshalReadRequest(MarshalReadRequest(want))
	if err != nil {
		t.Fatalf("UnmarshalReadRequest() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestReadResponse_RoundTrip(t *testing.T) {
	want := ReadResponse{
		Results: []QueryResult{
			{
				TimeSeries: []TimeSeries{
					{
						Labels:  []Label{{Name: "__name__", Value: "levelrail_cpu_percent"}},
						Samples: []Sample{{Value: 1.5, Timestamp: 1000}, {Value: 2.5, Timestamp: 2000}},
					},
				},
			},
		},
	}
	got, err := UnmarshalReadResponse(MarshalReadResponse(want))
	if err != nil {
		t.Fatalf("UnmarshalReadResponse() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestReadRequest_EmptyIsValid(t *testing.T) {
	got, err := UnmarshalReadRequest(MarshalReadRequest(ReadRequest{}))
	if err != nil {
		t.Fatalf("UnmarshalReadRequest() error = %v", err)
	}
	if len(got.Queries) != 0 {
		t.Errorf("Queries = %v, want empty", got.Queries)
	}
}

func TestUnmarshalReadRequest_UnknownFieldSkipped(t *testing.T) {
	// Simulates a real Prometheus client sending accepted_response_types
	// (field 2, not modeled by this package): a varint field this
	// decoder has never seen should be skipped, not rejected, so this
	// server stays forward-compatible with fields it doesn't need.
	var b []byte
	b = appendEmbedded(b, fieldReadRequestQueries, marshalQuery(Query{StartTimestampMs: 1, EndTimestampMs: 2}))
	b = protowire.AppendTag(b, 2, protowire.VarintType) // an unknown-to-this-package field 2
	b = protowire.AppendVarint(b, 0)

	got, err := UnmarshalReadRequest(b)
	if err != nil {
		t.Fatalf("UnmarshalReadRequest() with an unknown field error = %v, want it skipped cleanly", err)
	}
	if len(got.Queries) != 1 || got.Queries[0].StartTimestampMs != 1 {
		t.Errorf("Queries = %+v, want the one real query preserved despite the unknown field", got.Queries)
	}
}

func TestUnmarshalReadRequest_TruncatedBytes_Errors(t *testing.T) {
	full := MarshalReadRequest(ReadRequest{Queries: []Query{{StartTimestampMs: 1, EndTimestampMs: 2}}})
	truncated := full[:len(full)-1]
	if _, err := UnmarshalReadRequest(truncated); err == nil {
		t.Error("UnmarshalReadRequest(truncated) error = nil, want a decode error on malformed input")
	}
}
