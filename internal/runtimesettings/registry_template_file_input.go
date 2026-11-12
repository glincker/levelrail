package runtimesettings

// KeyTemplatesFileInputMaxBytes caps one config file an install form supplies.
const KeyTemplatesFileInputMaxBytes = "templates.file_input_max_bytes"

func init() {
	registerLimit(Definition{
		Key: KeyTemplatesFileInputMaxBytes, Category: CategoryDeploys, Default: "16384", Env: "APP_TEMPLATE_FILE_INPUT_MAX_BYTES", Apply: ApplyLive,
		Title:       "Templates: config file input size limit",
		Description: "Largest config file in bytes an install form may supply for a template file input, from 1024 to 65536. A bigger file is refused. A change applies to the next install.",
	}, 1024, 65536)
}
