package api

import (
	"fmt"
	"math/rand"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

type cronField struct {
	name  string
	check func(expr string) error
}

func pruneCheck(expr string) error {
	for _, d := range runtimesettings.Definitions() {
		if d.Key == runtimesettings.KeyImagesPruneSchedule {
			return d.CheckValue(expr)
		}
	}
	return fmt.Errorf("images.prune_schedule not registered")
}

func cronFields() []cronField {
	return []cronField{
		{"backup and volume schedule", func(e string) error {
			if validateBackupScheduleRequest(httptest.NewRecorder(), "bkt_1", e, 0, 0) {
				return nil
			}
			return fmt.Errorf("rejected")
		}},
		{"scheduled task", checkScheduleFires},
		{"app redeploy schedule", func(e string) error {
			if e == "" {
				return fmt.Errorf("empty means run_at instead")
			}
			return validateCronIfSet(e, time.UTC)
		}},
		{"deploy freeze window", func(e string) error {
			return deploy.ValidateFreezeWindow(store.DeployFreezeWindow{Cron: e, Duration: time.Hour})
		}},
		{"maintenance and upgrade window", func(e string) error {
			return alerting.MaintenanceWindow{Name: "w", Cron: e, Duration: time.Hour, Scope: alerting.ScopeAll}.Validate()
		}},
		{"images prune schedule", pruneCheck},
		{"pipeline on.schedule", func(e string) error {
			_, issues := pipeline.Validate([]byte("version: 1\non:\n  schedule: ['" + e + "']\njobs:\n  a:\n    image: x\n    steps:\n      - run: x\n"))
			if len(issues) > 0 {
				return fmt.Errorf("%v", issues)
			}
			return nil
		}},
	}
}

func TestEveryCronFieldRejectsAndAccepts(t *testing.T) {
	for _, f := range cronFields() {
		for _, bad := range []string{"0 0 30 2 *", "0 0 31 4 *", "0 0 31 6,9 *", "1 2 3 4 5 6", "nope", ""} {
			if f.check(bad) == nil {
				t.Errorf("%s accepted %q", f.name, bad)
			}
		}
		for _, ok := range []string{"0 3 * * *", "0 0 29 2 *"} {
			if err := f.check(ok); err != nil {
				t.Errorf("%s rejected %q: %v", f.name, ok, err)
			}
		}
	}
}

func TestValidatorAcceptsExactlyRealDates(t *testing.T) {
	rng := rand.New(rand.NewSource(215)) //nolint:gosec // fixed seed for a reproducible test
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for range 400 {
		day, month := rng.Intn(31)+1, rng.Intn(12)+1
		expr := fmt.Sprintf("0 0 %d %d *", day, month)
		exists := false
		for y := now.Year(); y <= now.Year()+8 && !exists; y++ {
			d := time.Date(y, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			exists = d.Month() == time.Month(month) && !d.Before(now)
		}
		if got := cronexpr.Validate(expr, time.UTC, now) == nil; got != exists {
			t.Fatalf("%s: accepted=%v, date exists=%v", expr, got, exists)
		}
	}
}
