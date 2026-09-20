package spec

// Git source deploy trigger modes (store.GitSource.TriggerMode): which
// pushes to a connected repo actually trigger a deploy.
const (
	// TriggerModePush deploys on every push to the git source's
	// configured branch. The default, matching pre-trigger-mode
	// behavior.
	TriggerModePush = "push"
	// TriggerModeRelease deploys only on a tag ref push, or (GitHub
	// only) a "release" webhook event with action "published".
	TriggerModeRelease = "release"
)
