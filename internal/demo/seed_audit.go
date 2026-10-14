package demo

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type auditAction struct {
	ability, method, path, action, client string
	app                                   bool
}

var auditActions = []auditAction{
	{"deploy", "POST", "/api/v1/apps/%s/deploys", "app.deploy", "dashboard", true},
	{"write", "PUT", "/api/v1/apps/%s/env", "app.env.update", "dashboard", true},
	{"deploy", "POST", "/api/v1/apps/%s/deploys/rollback", "app.rollback", "dashboard", true},
	{"write", "PUT", "/api/v1/apps/%s/placement", "app.placement.update", "dashboard", true},
	{"write", "POST", "/api/v1/apps/%s/domains", "app.domain.add", "dashboard", true},
	{"deploy", "POST", "/api/v1/apps/%s/restart", "app.restart", "cli", true},
	{"write", "POST", "/api/v1/databases/pg-main/backups", "database.backup.run", "dashboard", false},
	{"write", "POST", "/api/v1/alerts/rules", "alert.rule.create", "dashboard", false},
	{"root", "PUT", "/api/v1/teams/org_default/members", "team.member.update", "dashboard", false},
	{"root", "POST", "/api/v1/secrets/rotate", "secrets.rotate", "cli", false},
}

type auditActor struct{ typ, id, name string }

func (s *seeder) audit() error {
	actors := make([]auditActor, 0, len(userSpecs)+2)
	for _, u := range userSpecs[:3] {
		actors = append(actors, auditActor{"session", u.id, u.name})
	}
	actors = append(actors,
		auditActor{"token", "tok_demo_ci", "ci-deploy"},
		auditActor{"system", "system", "scheduler"},
	)
	for i := 0; i < 120; i++ {
		a := actors[s.rng.intn(len(actors))]
		p := auditActions[s.rng.intn(len(auditActions))]
		if a.typ == "system" {
			p = auditActions[0]
		}
		path := p.path
		if p.app {
			path = fmt.Sprintf(p.path, appSpecs[s.rng.intn(len(appSpecs))].name)
		}
		client := p.client
		if a.typ == "token" {
			client = "api"
		}
		if a.typ == "system" {
			client = "system"
		}
		status := 200
		if s.rng.intn(25) == 0 {
			status = 409
		}
		at := s.opts.Anchor.Add(-time.Duration(i+1) * time.Duration(historyDays*24*60/120) * time.Minute)
		e := store.AuditEntry{
			ID: fmt.Sprintf("audit_demo_%04d", i+1), ActorType: a.typ, ActorID: a.id, ActorName: a.name,
			Ability: p.ability, Method: p.method, Path: path, StatusCode: status,
			RemoteAddr: fmt.Sprintf("198.51.100.%d", 10+s.rng.intn(200)), CreatedAt: store.FormatAuditTime(at),
			ClientKind: client, Action: p.action,
		}
		if err := s.db.SaveAuditEntry(s.ctx, e); err != nil {
			return fmt.Errorf("demo: save audit entry: %w", err)
		}
	}
	return nil
}
