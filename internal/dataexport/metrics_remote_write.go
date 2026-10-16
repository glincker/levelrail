package dataexport

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/golang/snappy"
	"google.golang.org/protobuf/encoding/protowire"
)

// Remote write 1.0 messages are four small protobufs, so they are encoded by
// hand rather than pulling in the prometheus module for prompb.
const (
	rwFieldTimeseries = 1
	rwFieldLabels     = 1
	rwFieldSamples    = 2
	rwFieldLabelName  = 1
	rwFieldLabelValue = 2
	rwFieldSampleVal  = 1
	rwFieldSampleTS   = 2
)

// EncodeRemoteWrite renders series as a snappy block compressed
// prompb.WriteRequest. Series without a timestamp use now.
func EncodeRemoteWrite(series []Series, now time.Time) []byte {
	ordered := append([]Series(nil), series...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Name != ordered[j].Name {
			return ordered[i].Name < ordered[j].Name
		}
		return labelKey(ordered[i]) < labelKey(ordered[j])
	})
	var req []byte
	for _, s := range ordered {
		labels := append([]Label{{"__name__", s.Name}}, s.Labels...)
		sort.SliceStable(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
		var ts []byte
		for _, l := range labels {
			var lb []byte
			lb = protowire.AppendTag(lb, rwFieldLabelName, protowire.BytesType)
			lb = protowire.AppendString(lb, l.Name)
			lb = protowire.AppendTag(lb, rwFieldLabelValue, protowire.BytesType)
			lb = protowire.AppendString(lb, l.Value)
			ts = protowire.AppendTag(ts, rwFieldLabels, protowire.BytesType)
			ts = protowire.AppendBytes(ts, lb)
		}
		when := s.Time
		if when.IsZero() {
			when = now
		}
		var sm []byte
		sm = protowire.AppendTag(sm, rwFieldSampleVal, protowire.Fixed64Type)
		sm = protowire.AppendFixed64(sm, math.Float64bits(s.Value))
		sm = protowire.AppendTag(sm, rwFieldSampleTS, protowire.VarintType)
		sm = protowire.AppendVarint(sm, uint64(when.UnixMilli()))
		ts = protowire.AppendTag(ts, rwFieldSamples, protowire.BytesType)
		ts = protowire.AppendBytes(ts, sm)

		req = protowire.AppendTag(req, rwFieldTimeseries, protowire.BytesType)
		req = protowire.AppendBytes(req, ts)
	}
	return snappy.Encode(nil, req)
}

// RemoteWriteURL returns the endpoint unchanged: remote write receivers
// differ in path (/api/v1/push, /api/prom/push, /api/v1/write), so none is
// guessed.
func RemoteWriteURL(endpoint string) string { return endpoint }

func postRemoteWrite(ctx context.Context, client *http.Client, url string, headers map[string]string, body []byte) error {
	merged := mergeHeaders(headers, map[string]string{
		"Content-Encoding":                  "snappy",
		"X-Prometheus-Remote-Write-Version": "0.1.0",
	})
	return postBody(ctx, client, url, "application/x-protobuf", merged, body, "remote write")
}

// remoteWriteAuthHeaders turns the operator's key into an Authorization
// header: basic when a username is set, bearer otherwise.
func remoteWriteAuthHeaders(username, key string) map[string]string {
	if username != "" {
		return map[string]string{"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+key))}
	}
	return map[string]string{"Authorization": "Bearer " + key}
}

func validateRemoteWriteAuth(username string) error {
	if len(username) > 256 || strings.ContainsAny(username, ":\r\n") {
		return fmt.Errorf("username must be at most 256 characters without a colon")
	}
	return nil
}
