package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// Seeded read-path benchmarks, run by hand: go test -run xxx -bench HotPath ./internal/store
const (
	benchApps     = 400
	benchAttempts = 40000
	benchAudit    = 80000
)

func seedHotPaths(b *testing.B) *DB {
	b.Helper()
	db, err := Open(context.Background(), filepath.Join(b.TempDir(), "levelrail.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for i := 0; i < benchApps; i++ {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: fmt.Sprintf("app-%04d", i), Image: "nginx:1", Port: 80}); err != nil {
			b.Fatal(err)
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	base := time.Now().UTC().Add(-90 * 24 * time.Hour)
	statuses := []string{"succeeded", "succeeded", "succeeded", "failed", "superseded"}
	for i := 0; i < benchAttempts; i++ {
		st := base.Add(time.Duration(i) * (90 * 24 * time.Hour / benchAttempts))
		fin := st.Add(40 * time.Second).Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO deploy_attempts (id, service_name, image, status, started_at, finished_at, source, commit_sha, commit_message)
			VALUES (?, ?, ?, ?, ?, ?, 'webhook', ?, 'fix thing')`,
			fmt.Sprintf("dep_%08d", i), fmt.Sprintf("app-%04d", i%benchApps), fmt.Sprintf("img:%d", i%7), statuses[i%len(statuses)],
			st.Format(time.RFC3339Nano), fin, fmt.Sprintf("%040d", i))
		if err != nil {
			b.Fatal(err)
		}
	}
	for i := 0; i < benchAudit; i++ {
		ts := base.Add(time.Duration(i) * (90 * 24 * time.Hour / benchAudit))
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at, action)
			VALUES (?, 'session', ?, 'alice', 'write', 'POST', ?, ?, '10.0.0.1', ?, ?)`,
			fmt.Sprintf("aud_%08d", i), fmt.Sprintf("u%d", i%5), fmt.Sprintf("/api/v1/apps/app-%04d/deploy", i%benchApps),
			200+(i%9)*50*(i%2), FormatAuditTime(ts), fmt.Sprintf("app.deploy.%d", i%6))
		if err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	return db
}

func BenchmarkHotPath(b *testing.B) {
	db := seedHotPaths(b)
	ctx := context.Background()
	before := time.Now().Add(-time.Hour)
	run := func(name string, fn func() error) {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := fn(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	run("deployments_page", func() error { _, _, err := db.ListDeployments(ctx, DeploymentFilter{Limit: 50}); return err })
	run("deployments_app", func() error {
		_, _, err := db.ListDeployments(ctx, DeploymentFilter{Limit: 50, App: "app-0042"})
		return err
	})
	run("deployments_failed", func() error {
		_, _, err := db.ListDeployments(ctx, DeploymentFilter{Limit: 50, Statuses: []string{DeploymentFailed}})
		return err
	})
	run("deployment_briefs", func() error {
		_, err := db.ListDeploymentBriefs(ctx, time.Now().Add(-14*24*time.Hour), nil)
		return err
	})
	run("audit_page", func() error { _, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{}); return err })
	run("audit_before", func() error { _, err := db.ListAuditEntries(ctx, 50, &before, AuditEntryFilter{}); return err })
	run("audit_failed", func() error {
		_, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{FailedOnly: true})
		return err
	})
	run("audit_actor_type", func() error {
		_, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{ActorType: "token"})
		return err
	})
	run("audit_action", func() error {
		_, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{Action: "app.deploy.3"})
		return err
	})
	run("audit_path", func() error {
		_, err := db.ListAuditEntries(ctx, 50, nil, AuditEntryFilter{Path: "/api/v1/apps/app-0042/deploy"})
		return err
	})
	run("apps_page", func() error {
		_, _, err := db.ListDesiredServicesFiltered(ctx, AppListFilter{Limit: 50})
		return err
	})
	run("apps_search", func() error {
		_, _, err := db.ListDesiredServicesFiltered(ctx, AppListFilter{Limit: 50, Query: "app-04"})
		return err
	})
	run("app_names", func() error { _, err := db.ListAppNamesFiltered(ctx, AppListFilter{}); return err })
}
