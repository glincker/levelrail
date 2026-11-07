// Package stressprofile loads and validates the YAML profiles that drive the
// local stress harness, including the pass/fail budgets it judges a run by.
package stressprofile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvProfile names the profile file when no path is given on the command line.
const EnvProfile = "LEVELRAIL_STRESS_PROFILE"

// Profile describes one stress run: the estate to seed, the load to apply and the budgets to hold.
type Profile struct {
	Name             string        `yaml:"name"`
	Seed             uint64        `yaml:"seed"`
	Apps             int           `yaml:"apps"`
	Databases        int           `yaml:"databases"`
	Nodes            int           `yaml:"nodes"`
	SeededDeploys    int           `yaml:"seeded_deploys"`
	EnvsPerApp       int           `yaml:"envs_per_app"`
	DomainsPerApp    int           `yaml:"domains_per_app"`
	DeploysPerMinute int           `yaml:"deploys_per_minute"`
	ListClients      int           `yaml:"list_clients"`
	WatchClients     int           `yaml:"watch_clients"`
	LogLinesPerSec   int           `yaml:"log_lines_per_second"`
	Duration         time.Duration `yaml:"duration"`
	Thresholds       Thresholds    `yaml:"thresholds"`
}

// Thresholds are the budgets a run must stay within; every field is required.
type Thresholds struct {
	P95Ms              float64 `yaml:"p95_ms"`
	P99Ms              float64 `yaml:"p99_ms"`
	MaxRSSMB           float64 `yaml:"max_rss_mb"`
	MaxGoroutines      int     `yaml:"max_goroutines"`
	MaxSQLiteMB        float64 `yaml:"max_sqlite_mb"`
	MaxReconcilePassMs float64 `yaml:"max_reconcile_pass_ms"`
	MaxErrorRatePct    float64 `yaml:"max_error_rate_pct"`
}

// Parse decodes and validates a profile, rejecting unknown keys so a typo cannot silently drop a budget.
func Parse(data []byte) (Profile, error) {
	var p Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("stressprofile: decode: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Load reads a profile from path, or from the file named by LEVELRAIL_STRESS_PROFILE when path is empty.
func Load(path string) (Profile, error) {
	if path == "" {
		path = os.Getenv(EnvProfile)
	}
	if path == "" {
		return Profile{}, fmt.Errorf("stressprofile: no profile path given and %s is unset", EnvProfile)
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied profile path
	if err != nil {
		return Profile{}, fmt.Errorf("stressprofile: read %s: %w", path, err)
	}
	p, err := Parse(data)
	if err != nil {
		return Profile{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Validate reports every problem in the profile at once.
func (p Profile) Validate() error {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	if strings.TrimSpace(p.Name) == "" {
		bad("name is required")
	}
	if p.Apps < 1 {
		bad("apps must be at least 1")
	}
	if p.Nodes < 1 {
		bad("nodes must be at least 1")
	}
	counts := map[string]int{
		"databases": p.Databases, "seeded_deploys": p.SeededDeploys, "envs_per_app": p.EnvsPerApp,
		"domains_per_app": p.DomainsPerApp, "deploys_per_minute": p.DeploysPerMinute,
		"list_clients": p.ListClients, "watch_clients": p.WatchClients, "log_lines_per_second": p.LogLinesPerSec,
	}
	for name, v := range counts {
		if v < 0 {
			bad("%s must not be negative", name)
		}
	}
	if p.Duration < 5*time.Second {
		bad("duration must be at least 5s")
	}
	t := p.Thresholds
	if t.P95Ms <= 0 || t.P99Ms <= 0 || t.MaxRSSMB <= 0 || t.MaxGoroutines <= 0 || t.MaxSQLiteMB <= 0 || t.MaxReconcilePassMs <= 0 {
		bad("every threshold except max_error_rate_pct must be set above zero")
	}
	if t.MaxErrorRatePct < 0 || t.MaxErrorRatePct > 100 {
		bad("max_error_rate_pct must be between 0 and 100")
	}
	if t.P99Ms < t.P95Ms {
		bad("p99_ms must not be below p95_ms")
	}
	if len(errs) > 0 {
		return errors.New("stressprofile: invalid profile: " + strings.Join(errs, "; "))
	}
	return nil
}

// PhaseSplit divides Duration into the steady, churn and load windows.
func (p Profile) PhaseSplit() (steady, churn, load time.Duration) {
	steady = p.Duration / 5
	churn = p.Duration * 2 / 5
	load = p.Duration - steady - churn
	return steady, churn, load
}
