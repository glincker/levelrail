package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runWhoamiAccess prints "auth whoami --access": tier, teams, attached
// policies and a per-area allowed/denied list with the missing ability.
func runWhoamiAccess(client *apiclient.Client, stdout, stderr io.Writer, jsonOut bool, of outputFlags) int {
	access, err := client.MyAccess(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("whoami --access: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, access, func() {
		_, _ = fmt.Fprintf(stdout, "name:       %s (%s)\n", access.Name, access.Kind)
		_, _ = fmt.Fprintf(stdout, "tier:       %s\n", access.Tier)
		_, _ = fmt.Fprintf(stdout, "abilities:  %s\n", strings.Join(access.Abilities, ", "))
		teams := make([]string, 0, len(access.Teams))
		for _, t := range access.Teams {
			teams = append(teams, t.Name+" ("+t.Role+")")
		}
		_, _ = fmt.Fprintf(stdout, "teams:      %s\n", strings.Join(teams, ", "))
		policies := make([]string, 0, len(access.Policies))
		for _, p := range access.Policies {
			policies = append(policies, p.Name)
		}
		_, _ = fmt.Fprintf(stdout, "policies:   %s\n", strings.Join(policies, ", "))
		for _, c := range access.Capabilities {
			if c.Allowed {
				_, _ = fmt.Fprintf(stdout, "  can     %s\n", c.Key)
				continue
			}
			_, _ = fmt.Fprintf(stdout, "  cannot  %s: %s\n", c.Key, c.Reason)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
