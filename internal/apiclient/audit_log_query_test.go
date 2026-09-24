package apiclient

import "testing"

func TestAuditLogQuery_SearchAndFailed(t *testing.T) {
	tests := []struct {
		name string
		opts ListAuditLogOptions
		want string
	}{
		{"none", ListAuditLogOptions{}, ""},
		{"search", ListAuditLogOptions{Search: "a b"}, "q=a+b"},
		{"failed", ListAuditLogOptions{FailedOnly: true}, "status=failed"},
		{"both", ListAuditLogOptions{Search: "x", FailedOnly: true}, "q=x&status=failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auditLogQuery(tt.opts).Encode(); got != tt.want {
				t.Errorf("query = %q, want %q", got, tt.want)
			}
		})
	}
}
