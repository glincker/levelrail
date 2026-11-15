package runtimesettings

// KeyDeployOrphanGraceSeconds is how old a running deploy must be before cancel may treat it as orphaned.
const KeyDeployOrphanGraceSeconds = "deploy.orphan_grace_seconds"

func init() {
	registerLimit(Definition{
		Key: KeyDeployOrphanGraceSeconds, Category: CategoryDeploys, Default: "120", Env: "APP_DEPLOY_ORPHAN_GRACE_SECONDS", Apply: ApplyLive,
		Title:       "Orphaned deploy grace",
		Description: "Seconds a deploy marked running must have been going before cancel may finish it as orphaned when no build in this control plane owns it. It keeps a deploy that is just starting from being canceled by mistake. Between 10 and 3600; a change applies to the next cancel.",
	}, 10, 3600)
}
