package main

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// dashboardBaseURL is APP_DASHBOARD_URL, else APP_PUBLIC_HOST plus the
// dashboard port, else empty (no links are added).
func dashboardBaseURL() string {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_DASHBOARD_URL")), "/")
	if base != "" {
		return base
	}
	if host := publicHost(); host != "" {
		return "http://" + net.JoinHostPort(host, dashboardPort())
	}
	return ""
}

// alertDashboardLink maps an app to its alerts page for notification links.
func alertDashboardLink() func(app string) string {
	base := dashboardBaseURL()
	if base == "" {
		return nil
	}
	return func(app string) string {
		return base + "/apps/" + url.PathEscape(app) + "/alerts"
	}
}
