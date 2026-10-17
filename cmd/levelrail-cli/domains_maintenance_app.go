package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const appWideUsage = "apply to every domain of the app, all or none (takes only the app name)"

func maintenanceArgCount(appWide bool) int {
	if appWide {
		return 1
	}
	return 2
}

func maintenanceArgsDesc(appWide bool) string {
	if appWide {
		return "an app name"
	}
	return "an app name and a domain"
}

func printAppMaintenanceHuman(out io.Writer, m apiclient.AppMaintenanceResource) {
	_, _ = fmt.Fprintf(out, "app: %s\nmaintenance: %d of %d domains\n", m.App, m.Active, m.Total)
	for _, d := range m.Domains {
		state := "off"
		if d.Enabled {
			state = "on"
		}
		_, _ = fmt.Fprintf(out, "  %s: %s\n", d.Domain, state)
	}
	if len(m.Domains) > 0 && m.Enabled {
		printMaintenanceEffects(out, m.Domains[0].DomainMaintenanceResource)
	}
}

func appMaintenanceGet(stdout, stderr io.Writer, jsonOut bool, of outputFlags, c *Client, app string) int {
	m, err := c.GetAppMaintenance(context.Background(), app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get maintenance state for app %q: %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printAppMaintenanceHuman(stdout, m) })
}

func appMaintenanceSet(stdout, stderr io.Writer, jsonOut bool, of outputFlags, c *Client, app string, req apiclient.SetDomainMaintenanceRequest) int {
	m, err := c.SetAppMaintenance(context.Background(), app, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("enable maintenance mode for every domain of app %q (nothing was changed): %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printAppMaintenanceHuman(stdout, m) })
}

func appMaintenanceClear(stdout, stderr io.Writer, jsonOut bool, of outputFlags, c *Client, app string) int {
	m, err := c.ClearAppMaintenance(context.Background(), app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disable maintenance mode for app %q (retry to finish): %w", app, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, m, func() {
		_, _ = fmt.Fprintf(stdout, "maintenance mode disabled for every domain of app %q\n", app)
	})
}
