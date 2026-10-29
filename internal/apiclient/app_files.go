package apiclient

// AppFileResource mirrors internal/api's appFileResource; a secret file has
// no content and Redacted set.
type AppFileResource struct {
	Path      string `json:"path"`
	Mode      string `json:"mode"`
	Secret    bool   `json:"secret"`
	Redacted  bool   `json:"redacted"`
	SizeBytes int    `json:"size_bytes,omitempty"`
	Content   string `json:"content,omitempty"`
}

// AppFilesResource is GET and PUT /api/v1/apps/{name}/files' body.
type AppFilesResource struct {
	AppName string            `json:"app_name"`
	Files   []AppFileResource `json:"files"`
}

// SetAppFileRequest is PUT /api/v1/apps/{name}/files' body.
type SetAppFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
	Secret  bool   `json:"secret,omitempty"`
}
