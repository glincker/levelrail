package api

import (
	"context"
	"fmt"
	"os"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/fixit"
)

func (rt *Router) doctorCheckHardeningNow(context.Context) doctorCheckResource {
	info := docker.DetectRuntimeSocket(os.LookupEnv)
	if reporter, ok := rt.dockerPinger.(runtimeReporter); ok {
		info = reporter.Runtime()
	}
	cfg, err := docker.HardeningFromEnv(info)
	return doctorCheckContainerHardening(cfg, err)
}

func doctorFinding(c doctorCheckResource, command, reason string) fixit.Finding {
	f := fixit.Finding{Detail: c.Message}
	if c.Status == doctorStatusWarn || c.Status == doctorStatusFail {
		f.Failing, f.Command, f.Reason = true, command, reason
	}
	return f
}

// housekeepingFixes registers fixes for doctor checks that have a safe in-process
// action (auto) or one clear step elsewhere (guided).
func (rt *Router) housekeepingFixes() []fixit.Fix {
	fixes := []fixit.Fix{
		fixit.OrphansFix(fixit.OrphansDeps{
			Check: func(ctx context.Context) (fixit.CheckState, error) {
				c := rt.doctorCheckOrphans(ctx)
				if c.Status == doctorStatusUnknown {
					return fixit.CheckState{}, fmt.Errorf("orphan inventory unavailable: %s", c.Message)
				}
				return fixit.CheckState{Failing: c.Status == doctorStatusWarn, Detail: c.Message}, nil
			},
			Reclaimable: rt.fixitReclaimable,
			Prune:       rt.fixitPrune,
		}),
		fixit.GuidedCheckFix(fixit.GuidedDeps{
			ID: "secrets.stale.rotate", CheckID: "stale_secrets", Subject: "security",
			Title: "Rotate stale secrets",
			Why:   "Long-lived secret values widen the damage of a leak. Rotating needs the owning service to accept the new value, so it is done per secret.",
			Risk:  fixit.RiskSafe,
			Check: func(ctx context.Context) (fixit.Finding, error) {
				return doctorFinding(rt.doctorCheckStaleSecrets(ctx), "",
					"Open each app's Environment tab, update the secrets that have not changed in a long time, then re-check."), nil
			},
		}),
		fixit.GuidedCheckFix(fixit.GuidedDeps{
			ID: "hardening.container.enforce", CheckID: "container_hardening", Subject: "runtime",
			Title: "Enforce container hardening",
			Why:   "Dropping capabilities and blocking privilege escalation limits what a compromised container can do.",
			Risk:  fixit.RiskRestarts,
			Check: func(ctx context.Context) (fixit.Finding, error) {
				return doctorFinding(rt.doctorCheckHardeningNow(ctx), "",
					"The policy comes from APP_CONTAINER_HARDENING in the control plane service environment, so it cannot be changed from here. Unset it or set it to enforce, restart the service, then re-check."), nil
			},
		}),
		fixit.GuidedCheckFix(fixit.GuidedDeps{
			ID: "inodes.free", CheckID: "inodes", Subject: "disk",
			Title: "Free inodes on the data volume",
			Why:   "Running out of inodes stops writes even when free space remains.",
			Risk:  fixit.RiskSafe,
			Check: func(ctx context.Context) (fixit.Finding, error) {
				return doctorFinding(rt.doctorCheckInodes(ctx),
					"du --inodes -x "+rt.dataDir+" | sort -n | tail",
					"Find the directory holding the most small files, clear it, then re-check."), nil
			},
		}),
		fixit.GuidedCheckFix(fixit.GuidedDeps{
			ID: "disk.io.latency", CheckID: "disk_io_latency", Subject: "disk",
			Title: "Investigate slow disk writes",
			Why:   "Slow fsyncs slow every deploy and database on this host, and freeing space does not help.",
			Risk:  fixit.RiskSafe,
			Check: func(context.Context) (fixit.Finding, error) {
				return doctorFinding(rt.doctorCheckDiskIOLatency(), "iostat -x 2 3",
					"Check for a saturated disk, a network volume under load or a nearly full fragmented disk, then re-check."), nil
			},
		}),
	}
	if rt.authLib.tokens != nil {
		eng := rt.authLib.tokens
		fixes = append(fixes, fixit.LegacyTokensFix(fixit.LegacyTokensDeps{
			Pending: func(ctx context.Context) (int, error) {
				sum, err := eng.LegacyImportStatus(ctx)
				return sum.Pending, err
			},
			Migrate: func(ctx context.Context) error {
				_, err := eng.ImportLegacyTokens(ctx)
				return err
			},
		}))
	}
	return fixes
}
