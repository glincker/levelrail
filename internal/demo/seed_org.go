package demo

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/store"
)

type userSpec struct {
	id, email, name string
	abilities       []string
}

// Ability strings mirror internal/api's constants; duplicated here because
// the seeder must not import the HTTP layer.
var (
	abilitiesAdmin    = []string{"root"}
	abilitiesOperator = []string{"read", "read:sensitive", "write", "deploy"}
	abilitiesViewer   = []string{"read"}
)

var userSpecs = []userSpec{
	{"user_demo_ada", "ada.ortiz@acme.example", "Ada Ortiz", abilitiesAdmin},
	{"user_demo_ravi", "ravi.menon@acme.example", "Ravi Menon", abilitiesOperator},
	{"user_demo_lena", "lena.fischer@acme.example", "Lena Fischer", abilitiesOperator},
	{"user_demo_tom", "tom.aldridge@acme.example", "Tom Aldridge", abilitiesViewer},
	{"user_demo_mei", "mei.tanaka@acme.example", "Mei Tanaka", abilitiesViewer},
}

func (s *seeder) users() error {
	hash, err := bcrypt.GenerateFromPassword([]byte(VisitorPassword), bcrypt.MinCost)
	if err != nil {
		return fmt.Errorf("demo: hash visitor password: %w", err)
	}
	h := string(hash)
	visitor := store.User{
		ID: VisitorUserID, Email: VisitorUsername, DisplayName: "Alex Rivera",
		PasswordHash: &h, Abilities: []string{"read"}, CreatedAt: s.at(historyDays, 0),
	}
	// Teammates carry no password hash: they exist only so team and audit pages read as real.
	for i, u := range userSpecs {
		user := store.User{
			ID: u.id, Email: u.email, DisplayName: u.name, Abilities: u.abilities,
			IsFirstUser: i == 0, CreatedAt: s.at(historyDays, time.Duration(i)*time.Hour),
		}
		if err := s.db.CreateUser(s.ctx, user); err != nil {
			return fmt.Errorf("demo: create user %q: %w", u.id, err)
		}
	}
	if err := s.db.CreateUser(s.ctx, visitor); err != nil {
		return fmt.Errorf("demo: create visitor: %w", err)
	}
	return nil
}

type projectSpec struct{ id, name string }

var projectSpecs = []projectSpec{
	{"proj_storefront", "storefront"},
	{"proj_platform", "platform"},
	{"proj_internal_tools", "internal-tools"},
}

const orgID = "org_acme_cloud"

func envID(projectID, kind string) string { return "env_" + projectID[len("proj_"):] + "_" + kind }

func (s *seeder) organization() error {
	created := s.stamp(s.at(historyDays, 0))
	if err := s.db.SaveOrganization(s.ctx, store.Organization{ID: orgID, Name: "Acme Cloud", CreatedAt: created}); err != nil {
		return err
	}
	for _, p := range projectSpecs {
		if err := s.db.SaveProject(s.ctx, store.Project{ID: p.id, Name: p.name, CreatedAt: created}); err != nil {
			return err
		}
		if err := s.db.SetProjectOrganization(s.ctx, p.id, orgID); err != nil {
			return err
		}
		envs := []store.Environment{
			{ID: envID(p.id, "production"), ProjectID: p.id, Name: "production", Protected: true, CreatedAt: created, Kind: store.EnvironmentKindProduction, SortOrder: 0},
			{ID: envID(p.id, "staging"), ProjectID: p.id, Name: "staging", CreatedAt: created, Kind: store.EnvironmentKindTest, SortOrder: 1},
		}
		for _, e := range envs {
			if err := s.db.SaveEnvironment(s.ctx, e); err != nil {
				return err
			}
		}
	}
	return nil
}

type nodeSpec struct {
	id, name, addr string
	build, apps    bool
	status         store.NodeStatus
}

// Addresses are RFC 5737 documentation ranges so nothing real can leak.
var nodeSpecs = []nodeSpec{
	{"node_edge_fra_1", "edge-fra-1", "203.0.113.10:9443", false, true, store.NodeStatusOnline},
	{"node_worker_nyc_1", "worker-nyc-1", "203.0.113.21:9443", false, true, store.NodeStatusOnline},
	{"node_worker_sgp_1", "worker-sgp-1", "198.51.100.34:9443", false, true, store.NodeStatusOnline},
	{"node_build_nyc_1", "build-nyc-1", "203.0.113.45:9443", true, false, store.NodeStatusOnline},
}

func (s *seeder) nodes() error {
	for i, n := range nodeSpecs {
		joined := s.at(historyDays, time.Duration(i)*time.Hour)
		seen := s.opts.Anchor.Add(-time.Duration(10+i) * time.Second)
		notAfter := s.opts.Anchor.AddDate(0, 0, 60)
		node := store.Node{
			ID: n.id, Name: n.name, Address: n.addr, Status: n.status,
			CertFingerprint: "demo-" + n.id, CertSerial: fmt.Sprintf("%04d", i+1),
			JoinedAt: &joined, LastSeenAt: &seen, CertNotAfter: &notAfter,
			AcceptsAppWorkloads: n.apps, AcceptsBuildWorkloads: n.build,
			CreatedAt: joined, UpdatedAt: seen,
		}
		if err := s.db.SaveNode(s.ctx, node); err != nil {
			return fmt.Errorf("demo: save node %q: %w", n.id, err)
		}
	}
	return nil
}
