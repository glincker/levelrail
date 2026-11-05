package runtimesettings

// KeyOnboardingEnabled turns the first-run onboarding flow on or off for everyone.
const KeyOnboardingEnabled = "onboarding.enabled"

func init() {
	Register(Definition{
		Key: KeyOnboardingEnabled, Category: CategoryExperimental, Kind: KindBool, Default: "true",
		Env: "APP_ONBOARDING_ENABLED", Apply: ApplyLive,
		Title:       "Onboarding flow",
		Description: "Shows the guided welcome and demo flow to users who have not finished it. Turn it off to hide the flow for every user. Progress is kept, so turning it back on resumes where each user stopped.",
	})
}
