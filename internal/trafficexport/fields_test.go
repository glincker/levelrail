package trafficexport

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTruncateIP(t *testing.T) {
	tests := []struct{ in, want string }{
		{"203.0.113.77", "203.0.113.0"},
		{"2001:db8:abcd:1234:5678:9abc:def0:1234", "2001:db8:abcd::"},
		{"::ffff:203.0.113.77", "203.0.113.0"},
		{"fe80::1%eth0", "fe80::"},
		{"not-an-ip", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := TruncateIP(tt.in); got != tt.want {
			t.Errorf("TruncateIP(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func fullEvent() Event {
	return Event{
		App: "web", Host: "example.test", Method: "GET", Path: "/p", Status: 200, RequestID: "r1",
		ReferrerHost: "news.example", ClientIP: "203.0.113.77", UserAgent: "Mozilla/5.0 test", VisitorID: "vis123", UAFamily: "Firefox",
	}
}

func TestPolicyApply(t *testing.T) {
	tests := []struct {
		name                             string
		p                                Policy
		wantIP, wantUA, wantVis, wantRef string
	}{
		{"zero policy exports no identity", Policy{}, "", "", "", ""},
		{"truncate flag alone adds nothing", Policy{IPTruncate: true}, "", "", "", ""},
		{"referrer only", Policy{Fields: []string{FieldReferrerHost}}, "", "", "", "news.example"},
		{"raw ip", Policy{Fields: []string{FieldClientIP}}, "203.0.113.77", "", "", ""},
		{"truncated ip", Policy{Fields: []string{FieldClientIP}, IPTruncate: true}, "203.0.113.0", "", "", ""},
		{"ua only", Policy{Fields: []string{FieldUserAgent}}, "", "Mozilla/5.0 test", "", ""},
		{"visitor only", Policy{Fields: []string{FieldVisitorID}}, "", "", "vis123", ""},
		{"everything", Policy{Fields: OptionalFields()}, "203.0.113.77", "Mozilla/5.0 test", "vis123", "news.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Apply(fullEvent())
			if got.ClientIP != tt.wantIP || got.UserAgent != tt.wantUA || got.VisitorID != tt.wantVis || got.ReferrerHost != tt.wantRef {
				t.Errorf("got ip=%q ua=%q visitor=%q ref=%q", got.ClientIP, got.UserAgent, got.VisitorID, got.ReferrerHost)
			}
			if got.UAFamily != "" {
				t.Errorf("ua_family must never be exported, got %q", got.UAFamily)
			}
			if got.App != "web" || got.Path != "/p" || got.RequestID != "r1" {
				t.Errorf("non-optional fields changed: %+v", got)
			}
		})
	}
}

func TestPolicyApplyDefaultJSONHasNoIdentityKeys(t *testing.T) {
	b, err := json.Marshal(Policy{}.Apply(fullEvent()))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"203.0.113", "Mozilla", "vis123", "news.example", "client_ip", "user_agent", "visitor_id", "referrer_host", "Firefox"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("default record contains %q: %s", leak, b)
		}
	}
}

func TestValidateFieldsAndIdentity(t *testing.T) {
	if err := ValidateFields([]string{FieldClientIP, FieldReferrerHost}); err != nil {
		t.Errorf("valid fields rejected: %v", err)
	}
	for _, bad := range []string{"cookie", "authorization", "query", "body", ""} {
		if err := ValidateFields([]string{bad}); err == nil {
			t.Errorf("field %q accepted", bad)
		}
	}
	if HasIdentity([]string{FieldReferrerHost}) || !HasIdentity([]string{FieldReferrerHost, FieldUserAgent}) {
		t.Error("HasIdentity misclassifies the referrer or the user agent")
	}
}

func TestErrorKind(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{&StatusError{Sink: "x", Code: 401}, KindAuth},
		{&StatusError{Sink: "x", Code: 403}, KindAuth},
		{&StatusError{Sink: "x", Code: 429}, KindRate},
		{&StatusError{Sink: "x", Code: 400}, KindRejected},
		{&StatusError{Sink: "x", Code: 503}, KindServer},
		{errBlockedTarget, KindBlocked},
		{errors.New("boom"), KindOther},
	}
	for _, tt := range tests {
		if got := ErrorKind(tt.err); got != tt.want {
			t.Errorf("ErrorKind(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
