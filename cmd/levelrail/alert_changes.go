package main

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// alertDashboardLink maps an app to its alerts page for notification links.
// APP_DASHBOARD_URL wins; otherwise APP_PUBLIC_HOST plus the dashboard port
// is used, and with neither set no link is added.
func alertDashboardLink() func(app string) string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_DASHBOARD_URL")), "/")
	if base == "" {
		host := publicHost()
		if host == "" {
			return nil
		}
		base = "http://" + net.JoinHostPort(host, dashboardPort())
	}
	return func(app string) string {
		return base + "/apps/" + url.PathEscape(app) + "/alerts"
	}
}
