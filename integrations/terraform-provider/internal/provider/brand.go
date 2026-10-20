package provider

// The only file that names the product. Renaming the provider is one edit
// here plus the module path in go.mod.
const (
	// TypeName is the resource and data source prefix, e.g. levelrail_app.
	TypeName = "levelrail"
	// RegistryAddress is the address the provider is installed from.
	RegistryAddress = "registry.terraform.io/glincker/levelrail"
	// EnvURL and EnvToken are the fallbacks for the url and token settings.
	EnvURL   = "APP_API_URL"
	EnvToken = "APP_API_TOKEN"
)
