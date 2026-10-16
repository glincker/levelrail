package dataexport

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/snappy"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/GLINCKER/levelrail/internal/store"
)

type rwSeries struct {
	labels map[string]string
	value  float64
	ts     int64
}

func decodeRemoteWrite(t *testing.T, compressed []byte) []rwSeries {
	t.Helper()
	raw, err := snappy.Decode(nil, compressed)
	if err != nil {
		t.Fatalf("snappy decode: %v", err)
	}
	var out []rwSeries
	for len(raw) > 0 {
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 || num != 1 || typ != protowire.BytesType {
			t.Fatalf("bad WriteRequest tag %d %d", num, typ)
		}
		raw = raw[n:]
		tsBytes, n := protowire.ConsumeBytes(raw)
		raw = raw[n:]
		s := rwSeries{labels: map[string]string{}}
		for len(tsBytes) > 0 {
			num, _, n := protowire.ConsumeTag(tsBytes)
			tsBytes = tsBytes[n:]
			msg, n := protowire.ConsumeBytes(tsBytes)
			tsBytes = tsBytes[n:]
			switch num {
			case 1:
				var name, val string
				for len(msg) > 0 {
					f, _, n := protowire.ConsumeTag(msg)
					msg = msg[n:]
					v, n := protowire.ConsumeString(msg)
					msg = msg[n:]
					if f == 1 {
						name = v
					} else {
						val = v
					}
				}
				s.labels[name] = val
			case 2:
				for len(msg) > 0 {
					f, _, n := protowire.ConsumeTag(msg)
					msg = msg[n:]
					if f == 1 {
						v, n := protowire.ConsumeFixed64(msg)
						msg = msg[n:]
						s.value = math.Float64frombits(v)
					} else {
						v, n := protowire.ConsumeVarint(msg)
						msg = msg[n:]
						s.ts = int64(v) //nolint:gosec // test decode of a millisecond timestamp, far below int64 max
					}
				}
			}
		}
		out = append(out, s)
	}
	return out
}

func TestEncodeRemoteWrite(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 5_000_000, time.UTC)
	now := time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		series []Series
		want   []rwSeries
	}{
		{"empty", nil, nil},
		{
			"gauge with label and explicit time",
			[]Series{{Name: "plat_cpu_percent", Labels: []Label{{"app", "web"}}, Value: 12.5, Time: at}},
			[]rwSeries{{map[string]string{"__name__": "plat_cpu_percent", "app": "web"}, 12.5, at.UnixMilli()}},
		},
		{
			"zero time falls back to now, no labels",
			[]Series{{Name: "plat_apps", Value: 3}},
			[]rwSeries{{map[string]string{"__name__": "plat_apps"}, 3, now.UnixMilli()}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeRemoteWrite(t, EncodeRemoteWrite(tc.series, now))
			if len(got) != len(tc.want) {
				t.Fatalf("series = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i].value != tc.want[i].value || got[i].ts != tc.want[i].ts || len(got[i].labels) != len(tc.want[i].labels) {
					t.Fatalf("series %d = %+v, want %+v", i, got[i], tc.want[i])
				}
				for k, v := range tc.want[i].labels {
					if got[i].labels[k] != v {
						t.Fatalf("label %s = %q, want %q", k, got[i].labels[k], v)
					}
				}
			}
		})
	}
}

func TestRemoteWriteDestinationDelivers(t *testing.T) {
	var gotHeaders http.Header
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		in       Input
		wantAuth string
	}{
		{"basic", Input{Options: store.ExportOptions{Username: "1234"}, APIKey: "glc_key"},
			"Basic " + base64.StdEncoding.EncodeToString([]byte("1234:glc_key"))},
		{"bearer", Input{APIKey: "tok"}, "Bearer tok"},
		{"raw header", Input{Headers: map[string]string{"Authorization": "Custom x"}}, "Custom x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t, openDB(t), nil)
			in := tc.in
			in.Name, in.Kind, in.Endpoint = "rw", store.ExportKindMetricsRemoteWrite, srv.URL+"/api/v1/push"
			d, err := svc.Create(context.Background(), in)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			gotBody = nil
			if err := svc.Test(context.Background(), d.ID); err != nil {
				t.Fatalf("Test: %v", err)
			}
			for k, v := range map[string]string{
				"Content-Type":                      "application/x-protobuf",
				"Content-Encoding":                  "snappy",
				"X-Prometheus-Remote-Write-Version": "0.1.0",
				"Authorization":                     tc.wantAuth,
			} {
				if gotHeaders.Get(k) != v {
					t.Errorf("%s = %q, want %q", k, gotHeaders.Get(k), v)
				}
			}
			series := decodeRemoteWrite(t, gotBody)
			if len(series) == 0 || series[0].labels["__name__"] == "" {
				t.Fatalf("probe series = %+v", series)
			}
		})
	}
}

func TestRemoteWriteValidation(t *testing.T) {
	svc := newTestService(t, openDB(t), nil)
	bad := []Input{
		{Name: "a", Kind: store.ExportKindMetricsRemoteWrite, Endpoint: "ftp://x"},
		{Name: "a", Kind: store.ExportKindMetricsRemoteWrite, Endpoint: "http://x", MinLevel: "info"},
		{Name: "a", Kind: store.ExportKindMetricsRemoteWrite, Endpoint: "http://x", Options: store.ExportOptions{Username: "a:b"}},
		{Name: "a", Kind: store.ExportKindMetricsRemoteWrite, Endpoint: "http://x", Options: store.ExportOptions{IntervalSeconds: 5}},
		{Name: "a", Kind: store.ExportKindLogsHTTP, Endpoint: "http://x", Options: store.ExportOptions{Username: "u"}},
	}
	for i, in := range bad {
		if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: err = %v, want ErrInvalid", i, err)
		}
	}
	ok := Input{Name: "ok", Kind: store.ExportKindMetricsRemoteWrite, Endpoint: "http://127.0.0.1:9", Apps: []string{"web"}, APIKey: "k"}
	d, err := svc.Create(context.Background(), ok)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(d.Options.HeaderNames) != 1 || d.Options.HeaderNames[0] != "Authorization" {
		t.Fatalf("header names = %v", d.Options.HeaderNames)
	}
}
