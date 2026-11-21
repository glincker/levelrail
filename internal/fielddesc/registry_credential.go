package fielddesc

// RegistryCredential is the resource name of the private image registry login form.
const RegistryCredential = "registry-credentials" //nolint:gosec // resource name, not a credential

func init() {
	f := func(id string, t Type, req bool, flag, placeholder, doc string) Field {
		p := "integrations:fields.registryCredential." + id
		return Field{ID: id, Key: id, Type: t, LabelKey: p + ".label", HelpKey: p + ".help", Required: req, Secret: t == TypeSecret, Flag: flag, Placeholder: placeholder, Doc: doc}
	}
	Register(Resource{Name: RegistryCredential, Docs: Docs{File: "registry-credential-fields.md", Title: "Registry credential fields", Description: "Every field of a private registry credential with its type, requirement and CLI flag."}, Fields: []Field{
		f("name", TypeString, true, "name", "e.g. ghcr-bot", "display name, referenced from app.yaml build.registryCredential"),
		f("registry_host", TypeString, true, "registry-host", "ghcr.io", "registry hostname, for example ghcr.io"),
		f("username", TypeString, true, "username", "", "registry username"),
		f("password", TypeSecret, true, "password", "", "registry password or access token, write only"),
		f("expires_at", TypeString, false, "expires-at", "", "optional RFC3339 expiry the operator already knows, for example from a GitHub token"),
	}})
}
