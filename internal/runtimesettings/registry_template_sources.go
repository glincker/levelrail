package runtimesettings

// Settings for private template sources (git repos of templates).
const (
	KeyTemplatesSourcesEnabled        = "templates.sources_enabled"
	KeyTemplatesSourcesMaxMB          = "templates.sources_max_mb"
	KeyTemplatesSourcesMaxWireMB      = "templates.sources_max_wire_mb"
	KeyTemplatesSourcesMaxFiles       = "templates.sources_max_files"
	KeyTemplatesSourcesMaxDepth       = "templates.sources_max_depth"
	KeyTemplatesSourcesTimeoutSeconds = "templates.sources_timeout_seconds"
)

func init() {
	Register(Definition{
		Key: KeyTemplatesSourcesEnabled, Category: CategorySecurity, Kind: KindBool, Default: "false",
		Env: "APP_TEMPLATE_SOURCES_ENABLED", Apply: ApplyLive, Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Allow private template sources",
		Description: "Lets root add git repositories of templates over https or ssh to public hosts. Their templates pass the same validator and capability checks as built-in ones and are namespaced source/name. Off by default.",
	})
	Register(Definition{
		Key: KeyTemplatesSourcesMaxMB, Category: CategoryDeploys, Kind: KindInt, Default: "20",
		Env: "APP_TEMPLATE_SOURCES_MAX_MB", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1024),
		Title:       "Template source size cap",
		Description: "The most a template source may transfer and hold, in MB. A larger repository is refused.",
	})
	Register(Definition{
		Key: KeyTemplatesSourcesMaxWireMB, Category: CategoryDeploys, Kind: KindInt, Default: "1",
		Env: "APP_TEMPLATE_SOURCES_MAX_WIRE_MB", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1024),
		Title:       "Template source transfer cap",
		Description: "The most compressed data one template source sync may download, in MB. It bounds how far a hostile repository can inflate in memory.",
	})
	Register(Definition{
		Key: KeyTemplatesSourcesMaxFiles, Category: CategoryDeploys, Kind: KindInt, Default: "2000",
		Env: "APP_TEMPLATE_SOURCES_MAX_FILES", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100000),
		Title:       "Template source file cap",
		Description: "The most files a template source may contain. A repository with more is refused.",
	})
	Register(Definition{
		Key: KeyTemplatesSourcesMaxDepth, Category: CategoryDeploys, Kind: KindInt, Default: "6",
		Env: "APP_TEMPLATE_SOURCES_MAX_DEPTH", Apply: ApplyLive, CheckValue: intRangeCheck(1, 64),
		Title:       "Template source folder depth",
		Description: "The deepest folder nesting a template source may have. A deeper path refuses the sync.",
	})
	Register(Definition{
		Key: KeyTemplatesSourcesTimeoutSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "60",
		Env: "APP_TEMPLATE_SOURCES_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(5, 3600),
		Title:       "Template source sync timeout",
		Description: "How long one template source sync may run, in seconds, before it is cancelled.",
	})
}
