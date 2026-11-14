package runtimesettings

// Registered keys for compose references in the App kind (ADR 081).
const (
	KeyIaCComposeRefs       = "iac.compose_refs"
	KeyIaCComposeMaxFileKB  = "iac.compose_max_file_kb"
	KeyIaCComposeMaxFiles   = "iac.compose_max_files"
	KeyIaCComposeMaxTotalKB = "iac.compose_max_total_kb"
)

func init() {
	Register(Definition{
		Key: KeyIaCComposeRefs, Category: CategoryDeploys, Kind: KindBool, Default: "false", Env: "APP_IAC_COMPOSE_REFS", Apply: ApplyLive,
		Title:       "Compose references in stacks (beta)",
		Description: "Let an App document run from a compose file with spec.compose. Off refuses such documents. The next plan or apply uses a changed value.",
	})
	registerLimit(Definition{
		Key: KeyIaCComposeMaxFileKB, Category: CategoryDeploys, Default: "256", Env: "APP_IAC_COMPOSE_MAX_FILE_KB", Apply: ApplyLive,
		Title:       "Compose reference: largest file",
		Description: "Largest referenced compose file, in KiB. From 1 to 1024. The next plan uses a changed value.",
	}, 1, 1024)
	registerLimit(Definition{
		Key: KeyIaCComposeMaxFiles, Category: CategoryDeploys, Default: "20", Env: "APP_IAC_COMPOSE_MAX_FILES", Apply: ApplyLive,
		Title:       "Compose reference: files per request",
		Description: "Most compose files one plan or apply request may carry. From 1 to 200. The next plan uses a changed value.",
	}, 1, 200)
	registerLimit(Definition{
		Key: KeyIaCComposeMaxTotalKB, Category: CategoryDeploys, Default: "2048", Env: "APP_IAC_COMPOSE_MAX_TOTAL_KB", Apply: ApplyLive,
		Title:       "Compose reference: total size",
		Description: "Most compose bytes one request may carry in all, in KiB. From 1 to 6144, kept below the request body cap. The next plan uses a changed value.",
	}, 1, 6144)
}
