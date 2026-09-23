package build

// frameworkLabels maps a Railpack provider id (core.BuildResult.
// DetectedProviders[0]) to the human-readable name shown in the create-
// app wizard and stored on a deploy attempt. Scoped to exactly
// supportedRailpackProviders (railpack.go): a provider Railpack detects
// but this codebase cannot yet build is reported as "not detected"
// rather than a name the wizard could never actually build from.
var frameworkLabels = map[string]string{
	"node":   "Node.js",
	"golang": "Go",
	"java":   "Java (Spring Boot)",
	"python": "Python (Django)",
}

// FrameworkLabel returns provider's human-readable name and true, or
// ("", false) if provider is empty or outside supportedRailpackProviders.
func FrameworkLabel(provider string) (string, bool) {
	label, ok := frameworkLabels[provider]
	return label, ok
}
