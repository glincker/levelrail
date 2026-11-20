package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runBackupsDrillLatest implements "backups drill <app|target-id|control-plane>": drill the newest backup.
func runBackupsDrillLatest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c := newProtectionCmd(prog, "backups drill", backupsDrillUsage(prog), stdout, stderr, lookupEnv)
	var volume string
	var timeout time.Duration
	c.fs.StringVar(&volume, "volume", "", "volume of the app to drill (default: the app's only volume)")
	c.fs.DurationVar(&timeout, "timeout", 30*time.Minute, "how long to wait for a background drill")
	rest, exit, ok := c.parse(args, 1)
	if !ok {
		return exit
	}
	req := apiclient.DrillLatestRequest{App: rest[0], Volume: volume}
	switch {
	case rest[0] == "control-plane":
		req = apiclient.DrillLatestRequest{ControlPlane: true}
	case strings.HasPrefix(rest[0], "bkt_"):
		req = apiclient.DrillLatestRequest{TargetID: rest[0]}
	}
	ctx := context.Background()
	res, err := c.client.DrillLatestBackup(ctx, req)
	if err != nil {
		return c.fail(fmt.Errorf("drill %q: %w", rest[0], err))
	}
	if res.DrillID != "" {
		return c.waitDrill(ctx, res.DrillID, timeout)
	}
	code := c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "drill of %s: %s (%d ms)\n", res.BackupName, res.Status, res.DurationMS)
		for _, ch := range res.Checks {
			_, _ = fmt.Fprintf(stdout, "  %s: %t %s\n", ch.Name, ch.OK, ch.Detail)
		}
	})
	if code == exitOK && res.Status != "pass" {
		return exitCheckFailed
	}
	return code
}

func (c *protectionCmd) waitDrill(ctx context.Context, id string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for {
		d, err := c.client.GetBackupDrill(ctx, id)
		if err != nil {
			return c.fail(fmt.Errorf("read drill %q: %w", id, err))
		}
		if d.Status != "running" {
			code := c.render(d, func() { printDrill(c.stdout, d) })
			if code == exitOK && d.Status != "passed" {
				return exitCheckFailed
			}
			return code
		}
		if time.Now().After(deadline) {
			return c.fail(fmt.Errorf("drill %q is still running after %s", id, timeout))
		}
		time.Sleep(2 * time.Second)
	}
}
