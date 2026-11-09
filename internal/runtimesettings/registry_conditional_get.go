package runtimesettings

// KeyAPIConditionalGet switches ETag and 304 answers on read JSON endpoints.
const KeyAPIConditionalGet = "api.conditional_get"

// KeyAPIConditionalGetMaxBytes caps the body buffered to compute an ETag.
const KeyAPIConditionalGetMaxBytes = "api.conditional_get_max_bytes"

func init() {
	Register(Definition{
		Key: KeyAPIConditionalGet, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "APP_API_CONDITIONAL_GET", Apply: ApplyLive,
		Title:       "Conditional GET for read endpoints",
		Description: "Send an ETag on read JSON responses and answer a matching If-None-Match with 304 and no body. Secret-bearing routes never get an ETag. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeyAPIConditionalGetMaxBytes, Category: CategoryNetworking, Kind: KindInt, Default: "4194304",
		Env: "APP_API_CONDITIONAL_GET_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1024, 1<<30),
		Title:       "Conditional GET size cap",
		Description: "Largest read JSON body in bytes that still gets an ETag. Bigger responses stream without one. The next request uses a changed value.",
	})
}
