package importplan

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envrules/contract"
)

// The dotenv parser must accept and refuse the same keys as every other writer.
func TestParseEnvFileReportFollowsSharedFixture(t *testing.T) {
	contract.Run(t, 15, func(c contract.Case) (bool, bool) {
		if !c.DefaultPolicyOnly() || c.Options.Secret || len(c.Set) != 1 || len(c.Existing.Names) > 0 || len(c.Existing.Plain) > 0 || c.Options.ExistingUnknown {
			return false, true
		}
		for k, v := range c.Set {
			if k == "" || v == "" || strings.ContainsAny(k, "=\n\r \t#") || strings.ContainsAny(v, "\n\r\"'#") || strings.TrimSpace(k) != k {
				return false, true
			}
			vars, rep := ParseEnvFileReport(k+"="+v, ".env")
			return len(rep.Rejected) > 0 && len(vars) == 0, false
		}
		return false, true
	})
}

func TestParseEnvFileReportNamesRejectedLines(t *testing.T) {
	vars, rep := ParseEnvFileReport("OK=1\n1BAD=x\nnot a line\nLEVELRAIL_X=y\n", ".env")
	if len(vars) != 1 || len(rep.Accepted) != 1 || len(rep.Rejected) != 3 {
		t.Fatalf("vars=%d report=%+v", len(vars), rep)
	}
}
