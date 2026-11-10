package runtimesettings

// KeyDeployApprovalMaxRequired caps how many distinct approvers an
// environment may require, so a typo cannot lock deploys out.
const KeyDeployApprovalMaxRequired = "deploy.approval_max_required"

func init() {
	registerLimit(Definition{
		Key: KeyDeployApprovalMaxRequired, Category: CategoryDeploys, Default: "10", Env: "APP_DEPLOY_APPROVAL_MAX_REQUIRED", Apply: ApplyLive,
		Title:       "Most required approvers",
		Description: "Highest number of distinct approvers a protected environment may require before a deploy is released. From 1 to 50. An environment set above a lowered value uses this value at release time.",
	}, 1, 50)
}
