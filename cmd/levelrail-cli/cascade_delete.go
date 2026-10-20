package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const cascadeFlagUsage = "also delete every app and database inside (default: they keep running, detached)"

// deleteMaybeCascade runs the plain delete, or the cascading one that tears
// members down first. A partial cascade is an error: the container still
// exists and repeating the command resumes.
func deleteMaybeCascade(ctx context.Context, cascade bool, plain func(context.Context) error, deep func(context.Context) (apiclient.CascadeDeleteResult, error)) error {
	if !cascade {
		return plain(ctx)
	}
	res, err := deep(ctx)
	if err != nil {
		return err
	}
	if res.Status == "partial" {
		parts := make([]string, 0, len(res.Failed))
		for _, f := range res.Failed {
			parts = append(parts, fmt.Sprintf("%s %s: %s", f.Kind, f.Name, f.Error))
		}
		return fmt.Errorf("deleted %d app(s) and %d database(s) but not everything, run it again to resume: %s", len(res.DeletedApps), len(res.DeletedDatabases), strings.Join(parts, "; "))
	}
	return nil
}
