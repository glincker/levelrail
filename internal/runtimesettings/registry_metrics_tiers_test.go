package runtimesettings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func tierSvc(env map[string]string) *Service {
	return New(nil, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
}

func TestTierSetValidation(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		key  string
		val  string
		want string
	}{
		{"accept raw shorter", nil, KeyMetricsRetention, "200h", ""},
		{"accept 1m range", nil, KeyMetricsRange1m, "100h", ""},
		{"raw retention below range plus bucket", nil, KeyMetricsRetention, "6h", "plus one bucket"},
		{"raw range above retention", map[string]string{"APP_METRICS_RETENTION": "10h"}, KeyMetricsRawRange, "10h", "plus one bucket"},
		{"1m not longer than raw", nil, KeyMetricsRetention1m, "360h", "must be longer"},
		{"raw not shorter than 1m", nil, KeyMetricsRetention, "720h", "must be longer"},
		{"1h not longer than 1m", nil, KeyMetricsRetention1h, "720h", "must be longer"},
		{"1m below its range", map[string]string{"APP_METRICS_MINUTE_QUERY_MAX_RANGE": "700h"}, KeyMetricsRetention1m, "700h", "plus one bucket"},
		{"5m not longer than raw", nil, KeyMetricsRetention5m, "72h", "must be longer"},
		{"5m accepted when raw is short", map[string]string{"APP_METRICS_RETENTION": "6h"}, KeyMetricsRetention5m, "72h", ""},
		{"1d not longer than 1h", nil, KeyMetricsRetention1d, "8760h", "must be longer"},
		{"1d accepted", nil, KeyMetricsRetention1d, "9600h", ""},
		{"1h range at retention", map[string]string{"APP_METRICS_RETENTION_1H": "100h"}, KeyMetricsRange1h, "100h", "plus one bucket"},
	}
	for _, tc := range tests {
		raw, _ := json.Marshal(tc.val)
		_, err := tierSvc(tc.env).ValidateForWrite(context.Background(), tc.key, raw, WriteOptions{ConfirmWeakening: true})
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: unexpected %v", tc.name, err)
		case tc.want != "" && err == nil:
			t.Errorf("%s: accepted, want %q", tc.name, tc.want)
		case tc.want != "":
			var ve *ValidationError
			if !errors.As(err, &ve) || !strings.Contains(ve.Message, tc.want) {
				t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
			}
		}
	}
}

func TestTierSettingsApplyLive(t *testing.T) {
	env := map[string]string{}
	svc := tierSvc(env)
	ctx := context.Background()
	if got := svc.MetricsTiers(ctx).RawRange; got != 6*time.Hour {
		t.Fatalf("default raw range %s", got)
	}
	env["APP_METRICS_RAW_QUERY_MAX_RANGE"] = "2h"
	if got := svc.MetricsTiers(ctx).RawRange; got != 2*time.Hour {
		t.Errorf("changed raw range %s, want 2h", got)
	}
}

func TestTierCheckSkipsKeysTheServiceDoesNotDefine(t *testing.T) {
	d := defOf(t, KeyMetricsRetention)
	svc := NewWithDefinitions(nil, func(string) (string, bool) { return "", false }, []Definition{d})
	if _, err := svc.ValidateForWrite(context.Background(), KeyMetricsRetention, json.RawMessage(`"200h"`), WriteOptions{}); err != nil {
		t.Errorf("single-definition service rejected a valid save: %v", err)
	}
}

func TestTierInactiveDefaultsDoNotBlockActiveSaves(t *testing.T) {
	raw, _ := json.Marshal("500h")
	if _, err := tierSvc(nil).ValidateForWrite(context.Background(), KeyMetricsRetention, raw, WriteOptions{}); err != nil {
		t.Errorf("raw retention save blocked by an unset 5m default: %v", err)
	}
}
