package runtimesettings

// KeyBuildStageLogTailLines sizes the log tail kept for each build stage.
const KeyBuildStageLogTailLines = "build.stage_log_tail_lines"

func init() {
	registerLimit(Definition{
		Key: KeyBuildStageLogTailLines, Category: CategoryDeploys, Default: "200", Env: "APP_BUILD_STAGE_LOG_TAIL_LINES", Apply: ApplyLive,
		Title:       "Build stage log tail lines",
		Description: "How many of the last log lines each build stage keeps after the build ends, from 10 to 2000. The live stage view always shows output as it arrives; this only bounds what a finished build can replay per stage. The next build uses a changed value.",
	}, 10, 2000)
}
