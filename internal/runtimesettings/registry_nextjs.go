package runtimesettings

// KeyNextjsGenerateActionsKey turns on the one-time Server Actions key for wizard-created Next.js apps.
const KeyNextjsGenerateActionsKey = "nextjs.generate_actions_key"

// KeyNextjsMemoryFloorMB is the memory limit under which the doctor warns about Next.js apps.
const KeyNextjsMemoryFloorMB = "nextjs.memory_floor_mb"

func init() {
	Register(Definition{
		Key: KeyNextjsMemoryFloorMB, Category: CategoryDeploys, Kind: KindInt, Default: "256",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 65536),
		Title:       "Next.js: memory warning floor",
		Description: "The doctor warns when a Next.js app has a memory limit below this many MiB, because the server risks being killed for running out of memory. 0 turns the warning off. The warning is advisory and never blocks a deploy.",
	})

	// Safe to default on: it only adds a missing app secret for apps the wizard
	// detected as Next.js, never changes an existing key, and is off with one switch.
	Register(Definition{
		Key: KeyNextjsGenerateActionsKey, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "APP_NEXTJS_GENERATE_ACTIONS_KEY", Apply: ApplyLive,
		Title:       "Next.js: generate a Server Actions key",
		Description: "When the wizard deploys a Next.js app (version 14 or newer) that has no NEXT_SERVER_ACTIONS_ENCRYPTION_KEY, Levelrail generates a random key once, stores it as an app secret and gives it to every replica at runtime so Server Actions keep working across replicas and redeploys. Existing apps are never changed, and a redeploy never rotates the key. Delete the secret to rotate it. The key is not passed to builds.",
	})
}
