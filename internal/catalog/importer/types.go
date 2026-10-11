// Package importer converts upstream compose service templates into
// catalog manifests. It reads the upstream YAML as data only and emits
// manifests that go through the same loader and validators the shipped
// catalog uses.
package importer

import "github.com/GLINCKER/levelrail/internal/catalog"

// Reason codes explain why a template was skipped. They are stable: the
// committed import report and the docs page group by them.
const (
	ReasonUpstreamIgnored   = "upstream_ignored"
	ReasonParse             = "yaml_parse_error"
	ReasonBadID             = "bad_template_id"
	ReasonMissingMetadata   = "missing_metadata"
	ReasonBuildFromSource   = "build_from_source"
	ReasonHostAccess        = "host_access"
	ReasonHostBindMount     = "host_bind_mount"
	ReasonBindNeedsContent  = "bind_mount_needs_file_content"
	ReasonUnsupportedKey    = "unsupported_runtime_key"
	ReasonUnknownMagicVar   = "unknown_magic_var"
	ReasonNoDomainPort      = "no_domain_port"
	ReasonUnknownDomainKey  = "unknown_domain_key"
	ReasonUnresolvedImage   = "unresolved_image"
	ReasonUnsupportedTop    = "unsupported_top_level"
	ReasonPromptConflict    = "prompt_key_conflict"
	ReasonValidationFailed  = "validation_failed"
	ReasonUnsupportedVolume = "unsupported_volume"
	ReasonAlreadyInCatalog  = "already_in_catalog"
	ReasonUnsupportedProto  = "unsupported_protocol"
	ReasonNoHealthcheck     = "no_healthcheck"
)

// Note codes record what the converter changed or dropped on a template
// that was still imported.
const (
	NoteDroppedKey        = "dropped_key"
	NoteInlinedDefault    = "inlined_default"
	NoteBindToVolume      = "bind_mount_to_named_volume"
	NoteDroppedMount      = "dropped_host_mount"
	NoteUnpinnedImage     = "unpinned_image"
	NoteHostPortPublished = "host_port_published"
	NoteShellWrapped      = "shell_wrapped_command"
	NoteGeneratedAlias    = "generated_secret_alias"
	NoteDroppedLabel      = "dropped_labels"
	NoteTCPToHTTP         = "tcp_healthcheck_to_http"
	NoteHealthcheckAdded  = "healthcheck_added"
	NoteGeneratedBare     = "generated_bare_secret"
)

// Skip is the reason a template was not imported.
type Skip struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// Note is one recorded change on an imported template.
type Note struct {
	Code    string `json:"code"`
	Service string `json:"service,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Summary is the catalog-facing shape of an imported template, used by
// the report and the generated docs page.
type Summary struct {
	Category   string   `json:"category"`
	Services   []string `json:"services"`
	Ports      []string `json:"ports,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`
	Images     []string `json:"images"`
	Domains    []string `json:"domains,omitempty"`
	Generated  []string `json:"generated_secrets,omitempty"`
	UserPrompt []string `json:"prompts,omitempty"`
}

// Output is the result of converting one upstream template.
type Output struct {
	ID       string            `json:"id"`
	Imported bool              `json:"imported"`
	Skip     *Skip             `json:"skip,omitempty"`
	Notes    []Note            `json:"notes,omitempty"`
	Summary  *Summary          `json:"summary,omitempty"`
	Source   string            `json:"source"`
	Manifest *catalog.Manifest `json:"-"`
	Template []byte            `json:"-"`
	Compose  []byte            `json:"-"`
}

// Options carries per-run settings.
type Options struct {
	// SourcePath is the template's path inside the upstream repository.
	SourcePath string
	// SourceURL and License are recorded in the manifest's source block.
	SourceURL string
	License   string
}
