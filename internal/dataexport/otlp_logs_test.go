package dataexport

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestOTLPLogsURL(t *testing.T) {
	cases := map[string]string{
		"https://otel.example.com:4318":          "https://otel.example.com:4318/v1/logs",
		"https://otel.example.com:4318/":         "https://otel.example.com:4318/v1/logs",
		"https://otel.example.com/custom/v1/log": "https://otel.example.com/custom/v1/log",
	}
	for in, want := range cases {
		if got := OTLPLogsURL(in); got != want {
			t.Errorf("OTLPLogsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOTLPLogSenderPayload(t *testing.T) {
	s := &otlpLogSender{static: map[string]string{"env": "prod"}, scope: "plat"}
	at := time.Date(2026, 10, 14, 1, 2, 3, 0, time.UTC)
	body, err := s.payload([]LogRecord{
		{Time: at, Kind: "app", App: "web", Stream: "stderr", Level: "error", Message: "boom", Fields: json.RawMessage(`{"a":1}`)},
		{Time: at, Kind: "app", App: "web", Stream: "stdout", Level: "info", Message: "ok"},
		{Time: at, Kind: "app", App: "api", Stream: "stdout", Level: "warn", Message: "slow"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ResourceLogs []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeLogs []struct {
				LogRecords []struct {
					TimeUnixNano   string `json:"timeUnixNano"`
					SeverityNumber int    `json:"severityNumber"`
					SeverityText   string `json:"severityText"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(doc.ResourceLogs) != 2 {
		t.Fatalf("want one resource per app, got %d", len(doc.ResourceLogs))
	}
	web := doc.ResourceLogs[0]
	if web.Resource.Attributes[0].Value.StringValue != "web" || len(web.ScopeLogs[0].LogRecords) != 2 {
		t.Fatalf("unexpected web resource: %+v", web)
	}
	if last := web.Resource.Attributes[len(web.Resource.Attributes)-1]; last.Key != "env" || last.Value.StringValue != "prod" {
		t.Errorf("static label missing: %+v", last)
	}
	rec := web.ScopeLogs[0].LogRecords[0]
	if rec.SeverityNumber != 17 || rec.SeverityText != "ERROR" || rec.TimeUnixNano != "1791939723000000000" {
		t.Errorf("unexpected record: %+v", rec)
	}
}

func TestPlatformSeries(t *testing.T) {
	st := store.PlatformStats{
		ReconcileLagSeconds: 4, DeploysQueued: 2, DeploysRunning: 1, DeploysSucceeded: 5, DeploysFailed: 1,
		NodesByStatus: map[store.NodeStatus]int{store.NodeStatusOnline: 3},
	}
	got := map[string]float64{}
	for _, s := range PlatformSeries("p_", st, time.Now()) {
		key := s.Name
		for _, l := range s.Labels {
			key += "{" + l.Value + "}"
		}
		got[key] = s.Value
	}
	want := map[string]float64{
		"p_reconcile_lag_seconds": 4, "p_deploy_queue_depth": 2, "p_deploys_running": 1,
		"p_deploys_finished{succeeded}": 5, "p_deploys_finished{failed}": 1, "p_agent_connections": 3,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
}
