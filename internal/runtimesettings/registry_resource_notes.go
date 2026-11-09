package runtimesettings

// Registered caps for the description and notes an operator records on apps and databases.
const (
	KeyResourceDescriptionMaxChars = "resources.description_max_chars"
	KeyResourceNotesMaxBytes       = "resources.notes_max_bytes"
)

func init() {
	registerLimit(Definition{
		Key: KeyResourceDescriptionMaxChars, Category: CategoryDeploys, Default: "280", Env: "APP_RESOURCE_DESCRIPTION_MAX_CHARS", Apply: ApplyLive,
		Title:       "Description length",
		Description: "Most characters in the one line description of an app or database. From 1 to 2000. Saved descriptions are kept when the value is lowered.",
	}, 1, 2000)
	registerLimit(Definition{
		Key: KeyResourceNotesMaxBytes, Category: CategoryDeploys, Default: "65536", Env: "APP_RESOURCE_NOTES_MAX_BYTES", Apply: ApplyLive,
		Title:       "Notes size",
		Description: "Most bytes of markdown notes on an app or database. From 1024 to 1048576. Saved notes are kept when the value is lowered.",
	}, 1024, 1<<20)
}
