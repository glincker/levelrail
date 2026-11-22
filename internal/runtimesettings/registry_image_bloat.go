package runtimesettings

const (
	// KeyBuildImageBloatPercent is the image growth, against the previous build, that raises a bloat warning; 0 turns it off.
	KeyBuildImageBloatPercent = "build.image_bloat_percent"
	// KeyBuildImageBloatMinMB is the smallest growth in MiB that can raise a bloat warning.
	KeyBuildImageBloatMinMB = "build.image_bloat_min_mb"
)

func init() {
	registerLimit(Definition{
		Key: KeyBuildImageBloatPercent, Category: CategoryDeploys, Default: "30", Apply: ApplyLive,
		Title:       "Build: image bloat warning percent",
		Description: "Percent a built image may grow over the app's previous build before the Builds page and the deploy page warn and name the layer that grew, from 0 to 1000. 0 turns the warning off.",
	}, 0, 1000)
	registerLimit(Definition{
		Key: KeyBuildImageBloatMinMB, Category: CategoryDeploys, Default: "50", Apply: ApplyLive,
		Title:       "Build: image bloat minimum growth",
		Description: "MiB an image must grow before the bloat warning can fire, from 0 to 10240, so a small base image doing a normal jump stays quiet.",
	}, 0, 10240)
}
