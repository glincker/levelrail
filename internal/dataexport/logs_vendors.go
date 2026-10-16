package dataexport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// vendorPreset is what a vendor destination kind fills in so an operator
// only supplies a key: the endpoint, the header that carries the key and
// the payload encoding.
type vendorPreset struct {
	kind   store.ExportKind
	header string // header name that carries the key
	bearer bool   // send "Bearer <key>" rather than the bare key
}

var vendorPresets = map[store.ExportKind]vendorPreset{
	store.ExportKindLogsNewRelic:    {kind: store.ExportKindLogsNewRelic, header: "Api-Key"},
	store.ExportKindLogsDatadog:     {kind: store.ExportKindLogsDatadog, header: "DD-API-KEY"},
	store.ExportKindLogsAxiom:       {kind: store.ExportKindLogsAxiom, header: "Authorization", bearer: true},
	store.ExportKindLogsBetterStack: {kind: store.ExportKindLogsBetterStack, header: "Authorization", bearer: true},
}

// IsVendorLogKind reports whether k is a credentialed vendor destination.
func IsVendorLogKind(k store.ExportKind) bool {
	_, ok := vendorPresets[k]
	return ok
}

// hostFragment accepts a plain hostname value, so a region or site can never
// add a path or a second host to a preset URL.
func hostFragment(v string) bool {
	if v == "" || len(v) > 63 {
		return false
	}
	for _, r := range v {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' {
			return false
		}
	}
	return v[0] != '.' && v[0] != '-' && v[len(v)-1] != '.' && v[len(v)-1] != '-'
}

// VendorEndpoint is the ingest URL for a vendor kind and its options.
func VendorEndpoint(kind store.ExportKind, o store.ExportOptions) (string, error) {
	region := o.Region
	if region != "" && region != "us" && region != "eu" {
		return "", fmt.Errorf("options.region must be us or eu")
	}
	switch kind {
	case store.ExportKindLogsNewRelic:
		if region == "eu" {
			return "https://log-api.eu.newrelic.com/log/v1", nil
		}
		return "https://log-api.newrelic.com/log/v1", nil
	case store.ExportKindLogsDatadog:
		site := o.Site
		if site == "" {
			site = "datadoghq.com"
		}
		if !hostFragment(site) {
			return "", fmt.Errorf("options.site must be a plain hostname such as datadoghq.eu")
		}
		return "https://http-intake.logs." + site + "/api/v2/logs", nil
	case store.ExportKindLogsAxiom:
		if o.Dataset == "" {
			return "", fmt.Errorf("options.dataset is required for an axiom destination")
		}
		host := "api.axiom.co"
		if region == "eu" {
			host = "api.eu.axiom.co"
		}
		return "https://" + host + "/v1/datasets/" + url.PathEscape(o.Dataset) + "/ingest", nil
	case store.ExportKindLogsBetterStack:
		return "https://in.logs.betterstack.com", nil
	}
	return "", fmt.Errorf("%q is not a vendor destination kind", kind)
}

// vendorKeyHeaders turns the operator's key into the header the vendor reads.
func vendorKeyHeaders(kind store.ExportKind, key string) map[string]string {
	p := vendorPresets[kind]
	v := key
	if p.bearer {
		v = "Bearer " + key
	}
	return map[string]string{p.header: v}
}

// vendorLogSender posts one vendor specific JSON body per batch.
type vendorLogSender struct {
	url     string
	client  *http.Client
	headers map[string]string
	encode  func([]LogRecord, string) ([]byte, error)
	source  string
	what    string
}

func (s *vendorLogSender) send(ctx context.Context, recs []LogRecord) error {
	body, err := s.encode(recs, s.source)
	if err != nil {
		return permanent(fmt.Errorf("encode %s payload: %w", s.what, err))
	}
	return postBody(ctx, s.client, s.url, "application/json", s.headers, body, s.what)
}

func fieldsOf(r LogRecord) map[string]any {
	if len(r.Fields) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(r.Fields, &m) != nil {
		return nil
	}
	return m
}

func encodeNewRelicRecords(recs []LogRecord, source string) ([]byte, error) {
	type rec struct {
		Timestamp  int64          `json:"timestamp"`
		Message    string         `json:"message"`
		Attributes map[string]any `json:"attributes,omitempty"`
	}
	type block struct {
		Common map[string]any `json:"common"`
		Logs   []rec          `json:"logs"`
	}
	order := []string{}
	groups := map[string]*block{}
	for _, r := range recs {
		g, ok := groups[r.App]
		if !ok {
			g = &block{Common: map[string]any{"attributes": map[string]any{"service": r.App, "kind": r.Kind, "source": source}}}
			groups[r.App] = g
			order = append(order, r.App)
		}
		attrs := map[string]any{"stream": r.Stream, "level": r.Level}
		if f := fieldsOf(r); f != nil {
			attrs["fields"] = f
		}
		g.Logs = append(g.Logs, rec{Timestamp: r.Time.UnixMilli(), Message: r.Message, Attributes: attrs})
	}
	out := make([]block, 0, len(order))
	for _, app := range order {
		out = append(out, *groups[app])
	}
	return json.Marshal(out)
}

func encodeDatadogRecords(recs []LogRecord, source string) ([]byte, error) {
	type rec struct {
		DDSource string `json:"ddsource"`
		Service  string `json:"service"`
		Status   string `json:"status"`
		Message  string `json:"message"`
		Stream   string `json:"stream"`
		Time     string `json:"timestamp"`
	}
	out := make([]rec, 0, len(recs))
	for _, r := range recs {
		out = append(out, rec{DDSource: source, Service: r.App, Status: r.Level, Message: r.Message, Stream: r.Stream, Time: r.Time.UTC().Format(time.RFC3339Nano)})
	}
	return json.Marshal(out)
}

func encodeAxiomRecords(recs []LogRecord, _ string) ([]byte, error) {
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		m := map[string]any{
			"_time": r.Time.UTC().Format(time.RFC3339Nano), "message": r.Message,
			"stream": r.Stream, "level": r.Level, "service": r.App, "kind": r.Kind,
		}
		for k, v := range fieldsOf(r) {
			if _, taken := m[k]; !taken {
				m[k] = v
			}
		}
		out = append(out, m)
	}
	return json.Marshal(out)
}

func encodeBetterStackRecords(recs []LogRecord, _ string) ([]byte, error) {
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		out = append(out, map[string]any{
			"dt": r.Time.UTC().Format("2006-01-02 15:04:05.000 UTC"), "message": r.Message,
			"stream": r.Stream, "level": r.Level, "service": r.App,
		})
	}
	return json.Marshal(out)
}

func vendorEncoder(kind store.ExportKind) (func([]LogRecord, string) ([]byte, error), string) {
	switch kind {
	case store.ExportKindLogsNewRelic:
		return encodeNewRelicRecords, "newrelic logs"
	case store.ExportKindLogsDatadog:
		return encodeDatadogRecords, "datadog logs"
	case store.ExportKindLogsAxiom:
		return encodeAxiomRecords, "axiom logs"
	default:
		return encodeBetterStackRecords, "betterstack logs"
	}
}

func newVendorLogSender(d store.ExportDestination, client *http.Client, headers map[string]string, source string) *vendorLogSender {
	enc, what := vendorEncoder(d.Kind)
	return &vendorLogSender{url: d.Endpoint, client: client, headers: headers, encode: enc, source: strings.TrimSpace(source), what: what}
}
