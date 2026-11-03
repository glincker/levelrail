package runtimesettings

// KeyNextjsGenerateActionsKey turns on the one-time Server Actions key for wizard-created Next.js apps.
const KeyNextjsGenerateActionsKey = "nextjs.generate_actions_key"

func init() {
	// Safe to default on: it only adds a missing app secret for apps the wizard
	// detected as Next.js, never changes an existing key, and is off with one switch.
	Register(Definition{
		Key: KeyNextjsGenerateActionsKey, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "APP_NEXTJS_GENERATE_ACTIONS_KEY", Apply: ApplyLive,
		Title:       "Next.js: generate a Server Actions key",
		Description: "When the wizard deploys a Next.js app (version 14 or newer) that has no NEXT_SERVER_ACTIONS_ENCRYPTION_KEY, Levelrail generates a random key once, stores it as an app secret and gives it to every replica at runtime so Server Actions keep working across replicas and redeploys. Existing apps are never changed, and a redeploy never rotates the key. Delete the secret to rotate it. The key is not passed to builds.",
	})
}
