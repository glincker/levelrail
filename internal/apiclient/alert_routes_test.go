package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Each alert and status-page call must hit the documented method and path,
// with ids path-escaped and the query built only from set fields.
func TestAlertClientRoutes(t *testing.T) {
	var gotMethod, gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotURI = r.Method, r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")
	ctx := context.Background()

	cases := []struct {
		name   string
		call   func() error
		method string
		uri    string
	}{
		{"defaults list", func() error { _, e := c.ListDefaultAlerts(ctx); return e }, "GET", "/api/v1/alert-defaults"},
		{"defaults set", func() error { _, e := c.SetDefaultAlertEnabled(ctx, "a/b", true); return e }, "PUT", "/api/v1/alert-defaults/a%2Fb"},
		{"defaults reset", func() error { _, e := c.ResetDefaultAlerts(ctx); return e }, "POST", "/api/v1/alert-defaults/reset"},
		{"silences", func() error { _, e := c.ListAlertSilences(ctx, true); return e }, "GET", "/api/v1/alert-silences?include_expired=true"},
		{"silences live", func() error { _, e := c.ListAlertSilences(ctx, false); return e }, "GET", "/api/v1/alert-silences"},
		{"silence create", func() error { _, e := c.CreateAlertSilence(ctx, CreateSilenceRequest{Duration: "1h"}); return e }, "POST", "/api/v1/alert-silences"},
		{"silence expire", func() error { _, e := c.ExpireAlertSilence(ctx, "s1"); return e }, "DELETE", "/api/v1/alert-silences/s1"},
		{"silence rule", func() error { _, e := c.SilenceAlertRule(ctx, "web", "r1", "1h", "x"); return e }, "POST", "/api/v1/apps/web/alerts/r1/silence"},
		{"windows list", func() error { _, e := c.ListMaintenanceWindows(ctx); return e }, "GET", "/api/v1/alert-maintenance-windows"},
		{"windows create", func() error { _, e := c.CreateMaintenanceWindow(ctx, MaintenanceWindowResource{}); return e }, "POST", "/api/v1/alert-maintenance-windows"},
		{"windows update", func() error { _, e := c.UpdateMaintenanceWindow(ctx, "w1", MaintenanceWindowResource{}); return e }, "PUT", "/api/v1/alert-maintenance-windows/w1"},
		{"windows delete", func() error { return c.DeleteMaintenanceWindow(ctx, "w1") }, "DELETE", "/api/v1/alert-maintenance-windows/w1"},
		{"history", func() error {
			_, e := c.ListAlertHistory(ctx, AlertHistoryQuery{App: "web", Limit: 5, IncludeChanges: true})
			return e
		}, "GET", "/api/v1/alert-history?app=web&include=changes&limit=5"},
		{"history bare", func() error { _, e := c.ListAlertHistory(ctx, AlertHistoryQuery{}); return e }, "GET", "/api/v1/alert-history"},
		{"status get", func() error { _, e := c.GetStatusPage(ctx); return e }, "GET", "/api/v1/status-page"},
		{"status put", func() error { _, e := c.PutStatusPage(ctx, StatusPageSettings{}); return e }, "PUT", "/api/v1/status-page"},
		{"status comp list", func() error { _, e := c.ListStatusComponents(ctx); return e }, "GET", "/api/v1/status-page/components"},
		{"status comp del", func() error { return c.DeleteStatusComponent(ctx, "c1") }, "DELETE", "/api/v1/status-page/components/c1"},
		{"status inc list", func() error { _, e := c.ListStatusIncidents(ctx); return e }, "GET", "/api/v1/status-page/incidents"},
		{"status inc del", func() error { return c.DeleteStatusIncident(ctx, "i1") }, "DELETE", "/api/v1/status-page/incidents/i1"},
		{"templates", func() error { _, e := c.ListAlertTemplates(ctx); return e }, "GET", "/api/v1/alert-templates"},
		{"template apply", func() error { _, e := c.ApplyAlertTemplate(ctx, "web", "p", "ch"); return e }, "POST", "/api/v1/apps/web/alerts/apply-template"},
		{"quiet get", func() error { _, e := c.GetChannelQuietHours(ctx, "ch"); return e }, "GET", "/api/v1/notification-channels/ch/quiet-hours"},
		{"quiet set", func() error { _, e := c.SetChannelQuietHours(ctx, "ch", QuietHoursResource{}); return e }, "PUT", "/api/v1/notification-channels/ch/quiet-hours"},
		{"quiet clear", func() error { return c.ClearChannelQuietHours(ctx, "ch") }, "DELETE", "/api/v1/notification-channels/ch/quiet-hours"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("call error: %v", err)
			}
			if gotMethod != tc.method || gotURI != tc.uri {
				t.Errorf("got %s %s, want %s %s", gotMethod, gotURI, tc.method, tc.uri)
			}
		})
	}
}
