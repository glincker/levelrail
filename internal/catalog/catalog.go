// Package catalog holds Levelrail's own curated set of one-click
// service templates (ADR 015). Every Template's name, slogan, and
// Compose body here is written fresh for this platform: none of it is
// copied or lightly-rewritten from any other project's own dataset.
// Image tags are real, versioned tags for each well-known open-source
// project; where a specific tag's exact string couldn't be verified
// against a live registry in this environment, that entry says so in
// its own comment.
package catalog

// Template is one deployable entry in the catalog. Compose is a full
// compose.yaml body, valid against internal/compose's supported
// subset (see catalog_test.go, which parses and validates every one).
type Template struct {
	ID               string
	Name             string
	Slogan           string
	Category         string
	DocumentationURL string
	Compose          string
	// RecommendedMemoryBytes is a static, pre-deploy RAM advisory (zero
	// means none). Informational only, not checked against any node's
	// real available memory.
	RecommendedMemoryBytes int64
	// RequiresGPU marks entries that reserve an NVIDIA GPU through
	// deploy.resources.reservations.devices in their Compose body.
	RequiresGPU bool
}

// Templates is the full catalog, served by GET /api/v1/service-templates
// and GET /api/v1/service-templates/{id}. contribTemplates (contrib.go)
// are contributor-submitted YAML entries, merged in last so they're
// indistinguishable from the hand-written Go ones to every consumer.
var Templates = concat(
	starterKitsTemplates,
	automationTemplates,
	monitoringTemplates,
	storageTemplates,
	analyticsTemplates,
	infrastructureTemplates,
	securityTemplates,
	productivity1Templates,
	productivity2Templates,
	productivity3Templates,
	applicationsTemplates,
	devtools1Templates,
	devtools2Templates,
	dashboardTemplates,
	financeTemplates,
	media1Templates,
	media2Templates,
	communicationTemplates,
	databasesTemplates,
	iotTemplates,
	aiml1Templates,
	aiml2Templates,
	selfhosted1Templates,
	selfhosted2Templates,
	selfhosted3Templates,
	selfhosted4Templates,
	aiTemplates,
	devtoolsBatchTemplates,
	productivityBatchTemplates,
	automationBatchTemplates,
	catalogBatch2Templates,
	catalogBatch4Templates,
	contribTemplates,
)

// TemplateByID returns the Templates entry with this ID, or false if
// none matches.
func TemplateByID(id string) (Template, bool) {
	for _, tpl := range Templates {
		if tpl.ID == id {
			return tpl, true
		}
	}
	return Template{}, false
}

func concat(groups ...[]Template) []Template {
	var n int
	for _, g := range groups {
		n += len(g)
	}
	out := make([]Template, 0, n)
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
