package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const databasesRotatePasswordLabel = "databases rotate-password"

func runDatabasesRotatePassword(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var dryRun, status bool
	var everyDays int
	usage := "databases rotate-password <database> [--dry-run | --status | --every-days N] [flags]\n\n" +
		"Rotates the database owner password in the engine, verifies a real login,\n" +
		"switches the stored credential and restarts the linked apps. A failed\n" +
		"verification restores the old password. --dry-run lists the apps that would\n" +
		"restart. --every-days sets the automatic schedule (0 turns it off). A real\n" +
		"rotation asks for your password (and authenticator code)."
	c, code, ok := parseDBAccess(prog, databasesRotatePasswordLabel, usage, args, 1, "a database name", stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.BoolVar(&dryRun, "dry-run", false, "show what would restart without changing anything")
		fs.BoolVar(&status, "status", false, "show the last rotation, its age and the schedule")
		fs.IntVar(&everyDays, "every-days", -1, "rotate automatically every N days (0 turns it off)")
	})
	if !ok {
		return code
	}
	name := strings.TrimSpace(c.rest[0])
	ctx := context.Background()
	render := func(res apiclient.DatabasePasswordRotation) int {
		return c.result(res, func() { printDatabasePasswordRotation(stdout, res) })
	}
	switch {
	case everyDays >= 0:
		res, err := c.client.SetDatabasePasswordRotationInterval(ctx, name, everyDays)
		if err != nil {
			return c.fail(fmt.Errorf("set password rotation schedule for database %q: %w", name, err))
		}
		return render(res)
	case status:
		res, err := c.client.GetDatabasePasswordRotation(ctx, name)
		if err != nil {
			return c.fail(fmt.Errorf("get password rotation for database %q: %w", name, err))
		}
		return render(res)
	case dryRun:
		res, err := c.client.PreviewDatabasePasswordRotation(ctx, name)
		if err != nil {
			return c.fail(fmt.Errorf("preview password rotation for database %q: %w", name, err))
		}
		return render(res)
	}
	res, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.DatabasePasswordRotation, error) {
		return c.client.RotateDatabasePassword(ctx, name, p)
	})
	if err != nil {
		return c.fail(fmt.Errorf("rotate password of database %q: %w", name, err))
	}
	return render(res)
}

func printDatabasePasswordRotation(out io.Writer, r apiclient.DatabasePasswordRotation) {
	_, _ = fmt.Fprintf(out, "database:  %s (%s)\n", r.Database, r.Engine)
	if !r.Supported {
		_, _ = fmt.Fprintf(out, "rotation:  not available: %s\n", r.Reason)
		return
	}
	changed := "never recorded"
	if r.LastChangedAt != "" && r.AgeDays != nil {
		changed = fmt.Sprintf("%s (%d days ago)", r.LastChangedAt, *r.AgeDays)
	}
	_, _ = fmt.Fprintf(out, "changed:   %s\n", changed)
	_, _ = fmt.Fprintf(out, "state:     %s\n", r.State)
	if r.LastError != "" {
		_, _ = fmt.Fprintf(out, "last note: %s\n", r.LastError)
	}
	schedule := "off"
	if r.IntervalDays > 0 {
		schedule = fmt.Sprintf("every %d days, next %s", r.IntervalDays, r.NextDue)
	}
	_, _ = fmt.Fprintf(out, "schedule:  %s\n", schedule)
	if r.RotatedAt != "" {
		_, _ = fmt.Fprintf(out, "rotated:   %s\n", r.RotatedAt)
	}
	if len(r.RestartedApps) > 0 {
		_, _ = fmt.Fprintf(out, "restarted: %s\n", strings.Join(r.RestartedApps, ", "))
	} else if len(r.Apps) > 0 {
		_, _ = fmt.Fprintf(out, "will restart: %s\n", strings.Join(r.Apps, ", "))
	}
	if len(r.SuspendedApps) > 0 {
		_, _ = fmt.Fprintf(out, "suspended, pick it up on start: %s\n", strings.Join(r.SuspendedApps, ", "))
	}
}
