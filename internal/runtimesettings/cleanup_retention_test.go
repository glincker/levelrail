package runtimesettings

import (
	"context"
	"encoding/json"
	"testing"
)

func defOf(t *testing.T, key string) Definition {
	t.Helper()
	for _, d := range Definitions() {
		if d.Key == key {
			return d
		}
	}
	t.Fatalf("%s is not registered", key)
	return Definition{}
}

func TestDurationEnvToDays(t *testing.T) {
	tests := []struct {
		raw       string
		allowZero bool
		want      string
		wantErr   bool
	}{
		{"720h", false, "30", false},
		{"30d", false, "30", false},
		{"36h", false, "2", false},
		{"1h", false, "1", false},
		{"0", true, "0", false},
		{"0", false, "", true},
		{"-5h", true, "", true},
		{"soon", true, "", true},
	}
	for _, tc := range tests {
		got, err := DurationEnvToDays(tc.allowZero)(tc.raw)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%q: got %q, %v, want %q (err %v)", tc.raw, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestRetentionEnvOverridesKeepLegacyFormats(t *testing.T) {
	tests := []struct {
		key, env, val string
		want          int
	}{
		{KeyEventWebhookDays, "APP_EVENT_WEBHOOK_RETENTION", "720h", 30},
		{KeyEventWebhookDays, "APP_EVENT_WEBHOOK_RETENTION", "100h", 5},
		{KeyAlertHistoryDays, "APP_ALERT_HISTORY_RETENTION", "168h", 7},
		{KeyEmailLogDays, "APP_EMAIL_LOG_RETENTION_DAYS", "14", 14},
		{KeyEmailLogDays, "APP_EMAIL_LOG_RETENTION_DAYS", "0", 0},
		{KeyAuditLogDays, "APP_AUDIT_LOG_RETENTION_DAYS", "0", 90},
		{KeyAuditLogDays, "APP_AUDIT_LOG_RETENTION_DAYS", "14", 14},
		{KeyWebhookDeliveryDays, "APP_WEBHOOK_DELIVERY_RETENTION_DAYS", "bad", 30},
	}
	for _, tc := range tests {
		svc := New(nil, func(k string) (string, bool) { return tc.val, k == tc.env })
		if got := svc.Int(context.Background(), tc.key); got != tc.want {
			t.Errorf("%s=%s: got %d, want %d", tc.env, tc.val, got, tc.want)
		}
	}
}

func TestRetentionFloorsAndWeakening(t *testing.T) {
	audit := defOf(t, KeyAuditLogDays)
	sec := defOf(t, KeyAuditSecurityDays)
	for _, tc := range []struct {
		d       Definition
		raw     string
		wantErr bool
	}{
		{audit, "60", true}, {audit, "90", false}, {audit, "0", false},
		{sec, "180", true}, {sec, "365", false}, {sec, "0", false},
	} {
		_, err := ParseJSON(tc.d, json.RawMessage(tc.raw))
		if (err != nil) != tc.wantErr {
			t.Errorf("%s %s: err = %v", tc.d.Key, tc.raw, err)
		}
	}
	svc := New(nil, func(string) (string, bool) { return "", false })
	ctx := context.Background()
	for _, tc := range []struct {
		key, raw string
		want     bool
	}{
		{KeyAuditLogDays, "180", false},
		{KeyAuditLogDays, "90", false},
		{KeyAuditLogDays, "0", false},
		{KeyAuditSecurityDays, "365", false},
		{KeyAuditSecurityDays, "0", false},
	} {
		if got := svc.WeakensTo(ctx, tc.key, json.RawMessage(tc.raw)); got != tc.want {
			t.Errorf("%s -> %s weakens = %v, want %v", tc.key, tc.raw, got, tc.want)
		}
	}
	for _, tc := range []struct {
		cur, next string
		want      bool
	}{
		{"0", "90", true}, {"365", "180", true}, {"90", "365", false}, {"90", "0", false}, {"90", "90", false},
	} {
		if got := shorterDays(tc.cur, tc.next); got != tc.want {
			t.Errorf("shorterDays(%s,%s) = %v", tc.cur, tc.next, got)
		}
	}
}

func TestStringSetting(t *testing.T) {
	d := Definition{Key: "test.string", Kind: KindString, Default: "preview", AllowedStrings: []string{"off", "preview", "on"}}
	svc := NewWithDefinitions(nil, func(string) (string, bool) { return "", false }, []Definition{d})
	if got := svc.String(context.Background(), "test.string"); got != "preview" {
		t.Fatalf("default = %q", got)
	}
	if _, err := ParseJSON(d, json.RawMessage(`"maybe"`)); err == nil {
		t.Error("an unlisted string value was accepted")
	}
	if c, err := ParseJSON(d, json.RawMessage(`"on"`)); err != nil || c != "on" {
		t.Errorf("on = %q, %v", c, err)
	}
}

func TestCleanupIntervalMinimum(t *testing.T) {
	d := defOf(t, KeyCleanupInterval)
	if _, err := ParseJSON(d, json.RawMessage(`"5m"`)); err == nil {
		t.Error("5m is below the minimum")
	}
	if c, err := ParseJSON(d, json.RawMessage(`"15m"`)); err != nil || c != "15m0s" {
		t.Errorf("15m = %q, %v", c, err)
	}
}
