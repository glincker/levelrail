package fixit

import (
	"context"
	"fmt"
)

// NextjsFacts are the per-app observations the Next.js doctor checks read.
// The caller fills them from the app spec, build output and linked services.
type NextjsFacts struct {
	// App is the app name, used as the fix subject.
	App string
	// Standalone is true when the build produced the standalone server.
	Standalone bool
	// StaticCopied is true when .next/static and public sit beside the server.
	StaticCopied bool
	// SharpAvailable is true when the image optimizer can load sharp.
	SharpAvailable bool
	// ActionsKeySet is true when the Server Actions key reaches the runtime.
	ActionsKeySet bool
	// MemoryMB is the app memory limit; zero means unlimited.
	MemoryMB int
	// MinMemoryMB is the floor to warn under, supplied from settings or env.
	MinMemoryMB int
	// Replicas is the desired replica count.
	Replicas int
	// SharedCache is true when a Redis cache handler is linked.
	SharedCache bool
}

// NextjsDeps loads the facts for one app; found is false when it is not Next.js.
type NextjsDeps struct {
	Facts func(ctx context.Context, app string) (facts NextjsFacts, found bool, err error)
}

// NextjsFixes returns one guided fix per Next.js check for an app. None of
// them change the app: each shows what to do and re-checks afterwards.
func NextjsFixes(app string, d NextjsDeps) []Fix {
	mk := func(id, check, title, why, reason string, failing func(NextjsFacts) (bool, string)) Fix {
		return GuidedCheckFix(GuidedDeps{
			ID: id, CheckID: check, Subject: app, Title: title, Why: why, Risk: RiskSafe,
			Check: func(ctx context.Context) (Finding, error) {
				f, found, err := d.Facts(ctx, app)
				if err != nil {
					return Finding{}, err
				}
				if !found {
					return Finding{}, nil
				}
				bad, detail := failing(f)
				return Finding{Failing: bad, Detail: detail, Reason: reason}, nil
			},
		})
	}
	return []Fix{
		mk("nextjs.output.standalone", "nextjs_output", "Build Next.js in standalone mode",
			"Without standalone output the image is larger and starts slower.",
			"Set output: 'standalone' in next.config and redeploy.",
			func(f NextjsFacts) (bool, string) {
				return !f.Standalone, "The build is not in standalone mode."
			}),
		mk("nextjs.static.copy", "nextjs_static", "Copy static files into the image",
			"The standalone server does not serve .next/static or public on its own.",
			"Make sure the build copies .next/static and public next to server.js, then redeploy.",
			func(f NextjsFacts) (bool, string) {
				return f.Standalone && !f.StaticCopied, "Static files or public are missing from the image."
			}),
		mk("nextjs.sharp.available", "nextjs_sharp", "Add sharp for image optimization",
			"Without sharp, next/image falls back to a slower path or fails.",
			"Add sharp as a dependency and redeploy.",
			func(f NextjsFacts) (bool, string) {
				return !f.SharpAvailable, "sharp is not available at runtime."
			}),
		mk("nextjs.actions.key", "nextjs_actions_key", "Set the Server Actions key",
			"Without a fixed key, each replica and each deploy encrypts actions differently.",
			"Set NEXT_SERVER_ACTIONS_ENCRYPTION_KEY as a runtime secret and redeploy.",
			func(f NextjsFacts) (bool, string) {
				return !f.ActionsKeySet, "No Server Actions key is set."
			}),
		mk("nextjs.memory.floor", "nextjs_memory", "Raise the memory limit",
			"Next.js can be killed for running out of memory under a very low limit.",
			"Raise the app memory limit in its settings.",
			func(f NextjsFacts) (bool, string) {
				if f.MinMemoryMB <= 0 || f.MemoryMB <= 0 || f.MemoryMB >= f.MinMemoryMB {
					return false, ""
				}
				return true, fmt.Sprintf("Memory is %d MB, below the %d MB floor.", f.MemoryMB, f.MinMemoryMB)
			}),
		// Guided, not auto: linking a Redis also needs a cache handler in the app code.
		mk("nextjs.cache.shared", "nextjs_shared_cache", "Link a Redis for the shared cache",
			"With more than one replica, each keeps its own page cache and they drift apart.",
			"Add a Redis service from the catalog, link it to this app, and use a Redis cache handler in next.config.",
			func(f NextjsFacts) (bool, string) {
				return f.Replicas > 1 && !f.SharedCache, fmt.Sprintf("%d replicas run without a shared cache.", f.Replicas)
			}),
	}
}
