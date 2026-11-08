package runtimesettings

// KeyAPIConditionalGet switches ETag and 304 answers on read JSON endpoints.
const KeyAPIConditionalGet = "api.conditional_get"

func init() {
	Register(Definition{
		Key: KeyAPIConditionalGet, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "APP_API_CONDITIONAL_GET", Apply: ApplyLive,
		Title:       "Conditional GET for read endpoints",
		Description: "Send an ETag on read JSON responses and answer a matching If-None-Match with 304 and no body. Secret-bearing routes never get an ETag. The next request uses a changed value.",
	})
}
