package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envrules"
	"github.com/GLINCKER/levelrail/internal/envrules/contract"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Each writer below must refuse and accept exactly what the shared fixture says.
func TestEnvWritersFollowSharedFixture(t *testing.T) {
	rt := &Router{}
	ctx := context.Background()
	plain := func(c contract.Case) bool {
		// JSON bodies keep a trailing CR byte for byte, so the crlf case is not theirs.
		for _, w := range c.WantErrors {
			if w == "crlf" {
				return false
			}
		}
		return c.DefaultPolicyOnly() && !c.Options.Secret && len(c.Existing.Names) == 0 && len(c.Existing.Plain) == 0 && !c.Options.ExistingUnknown && !c.Options.Multiline
	}
	t.Run("template install env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) {
				return false, true
			}
			return rt.templateEnvRefusal(ctx, c.Set) != "", false
		})
	})
	t.Run("bundle import env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) {
				return false, true
			}
			return rt.bundleEnvRefusal(ctx, c.Set, nil) != "", false
		})
	})
	t.Run("bulk set env", func(t *testing.T) {
		contract.Run(t, 20, func(c contract.Case) (bool, bool) {
			if !plain(c) || len(c.Set) != 1 {
				return false, true
			}
			for k, v := range c.Set {
				return rt.bulkEnvRefusal(ctx, k, v, &store.DesiredService{}) != "", false
			}
			return false, true
		})
	})
}

func TestBuildReportNamesEveryRejectedKey(t *testing.T) {
	set := map[string]string{"GOOD": "1", "1BAD": "x", "LEVELRAIL_X": "y"}
	rep := envrules.BuildReport(set, envrules.Validate(set, envrules.Existing{}, envrules.DefaultPolicy(), envrules.Options{Multiline: true}))
	if len(rep.Accepted) != 1 || rep.Accepted[0] != "GOOD" || len(rep.Rejected) != 2 {
		t.Fatalf("report = %+v", rep)
	}
}
