package runtimesettings

// KeyPreviewSkipDrafts skips preview deploys for draft pull requests.
const KeyPreviewSkipDrafts = "deploy.preview_skip_drafts"

func init() {
	Register(Definition{
		Key: KeyPreviewSkipDrafts, Category: CategoryDeploys, Kind: KindBool, Default: "false", Apply: ApplyLive,
		Title:       "Skip previews for draft pull requests",
		Description: "When on, a draft pull request or merge request gets no preview build until it is marked ready for review. Off by default. Read on every pull request event.",
	})
}
