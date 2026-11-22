package runtimesettings

// Keys for the deploy duration regression alert.
const (
	KeyAlertsDurationRegressionPercent  = "alerts.duration_regression_percent"
	KeyAlertsDurationRegressionRecent   = "alerts.duration_regression_recent"
	KeyAlertsDurationRegressionBaseline = "alerts.duration_regression_baseline"
)

func init() {
	Register(Definition{
		Key: KeyAlertsDurationRegressionPercent, Category: CategoryObservability, Kind: KindInt, Default: "50",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, 1000),
		Title:       "Flag a build or deploy this percent slower than usual",
		Description: "A deploy_duration_regression alert fires when an app's recent build or deploy p50 grows past this percent over its own earlier p50, naming the stage that grew. 0 turns the check off.",
	})
	Register(Definition{
		Key: KeyAlertsDurationRegressionRecent, Category: CategoryObservability, Kind: KindInt, Default: "3",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, 30),
		Title:       "Newest runs used for the current duration",
		Description: "How many of the newest succeeded builds or deploys of an app form its current p50 for the duration regression alert.",
	})
	Register(Definition{
		Key: KeyAlertsDurationRegressionBaseline, Category: CategoryObservability, Kind: KindInt, Default: "10",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "Earlier runs used for the usual duration",
		Description: "How many succeeded builds or deploys before the newest ones set an app's usual p50 for the duration regression alert. An app needs recent plus this many runs before it is checked.",
	})
}
