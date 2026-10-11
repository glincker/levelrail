package apiclient

import (
	"context"
	"net/http"
)

// ImportAnalyzeRequest is POST /api/v1/imports/analyze's body.
type ImportAnalyzeRequest struct {
	RepoURL        string `json:"repo_url"`
	Ref            string `json:"ref,omitempty"`
	BaseDirectory  string `json:"base_directory,omitempty"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BuildType      string `json:"build_type,omitempty"`
	Name           string `json:"name,omitempty"`
	Token          string `json:"token,omitempty"`
}

// AnalyzeText mirrors internal/appwizard's Text.
type AnalyzeText struct {
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

// AnalyzeBuildSuggestion mirrors internal/repolayout's Suggestion.
type AnalyzeBuildSuggestion struct {
	BuildType      string `json:"build_type"`
	DockerfilePath string `json:"dockerfile_path,omitempty"`
	BaseDirectory  string `json:"base_directory,omitempty"`
	Reason         string `json:"reason"`
	ReasonCode     string `json:"reason_code"`
	Recommended    bool   `json:"recommended"`
}

// AnalyzeBuild mirrors internal/appwizard's Build.
type AnalyzeBuild struct {
	Type           string                   `json:"type"`
	DockerfilePath string                   `json:"dockerfile_path,omitempty"`
	BaseDirectory  string                   `json:"base_directory,omitempty"`
	Evidence       string                   `json:"evidence"`
	Alternatives   []AnalyzeBuildSuggestion `json:"alternatives"`
}

// AnalyzeEnvKey mirrors internal/appwizard's EnvKey. It never carries a value.
type AnalyzeEnvKey struct {
	Key        string `json:"key"`
	Required   bool   `json:"required"`
	Secret     bool   `json:"secret"`
	Source     string `json:"source"`
	ProvidedBy string `json:"provided_by,omitempty"`
	HasDefault bool   `json:"has_default"`
}

// AnalyzeDatabase mirrors internal/appwizard's DatabaseNeed.
type AnalyzeDatabase struct {
	Engine        string `json:"engine"`
	Version       string `json:"version"`
	SuggestedName string `json:"suggested_name"`
	EnvVar        string `json:"env_var"`
	Field         string `json:"field"`
	Evidence      string `json:"evidence"`
}

// ImportAnalysis mirrors internal/appwizard's Analysis.
type ImportAnalysis struct {
	RepoURL      string      `json:"repo_url"`
	Branch       string      `json:"branch"`
	Monorepo     bool        `json:"monorepo"`
	AppDirectory string      `json:"app_directory,omitempty"`
	Name         AnalyzeText `json:"name"`
	Stack        struct {
		Runtime   string `json:"runtime"`
		Framework string `json:"framework,omitempty"`
		Evidence  string `json:"evidence"`
	} `json:"stack"`
	Build AnalyzeBuild `json:"build"`
	Port  struct {
		Value    int    `json:"value"`
		Evidence string `json:"evidence"`
	} `json:"port"`
	Health AnalyzeText `json:"health"`
	Memory struct {
		Bytes    int64  `json:"bytes"`
		Evidence string `json:"evidence"`
	} `json:"memory"`
	Domain    AnalyzeText       `json:"domain"`
	Env       []AnalyzeEnvKey   `json:"env"`
	Databases []AnalyzeDatabase `json:"databases"`
	Warnings  []ImportWarning   `json:"warnings"`
	FilesRead int               `json:"files_read"`
}

// AnalyzeImport calls POST /api/v1/imports/analyze. It creates nothing.
func (c *Client) AnalyzeImport(ctx context.Context, req ImportAnalyzeRequest) (ImportAnalysis, error) {
	var out ImportAnalysis
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/analyze", req, &out)
	return out, err
}
