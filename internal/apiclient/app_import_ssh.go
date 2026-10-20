package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// AppImportSSH is the SSH login of a docker-ssh import source. The control
// plane holds it in memory only and uses it as a read-only transport to the
// old host's Docker socket.
type AppImportSSH struct {
	Host            string `json:"host"`
	Port            int    `json:"port,omitempty"`
	User            string `json:"user"`
	PrivateKey      string `json:"private_key,omitempty"`
	Passphrase      string `json:"passphrase,omitempty"`
	Password        string `json:"password,omitempty"`
	Fingerprint     string `json:"host_key_fingerprint,omitempty"`
	InsecureHostKey bool   `json:"insecure_host_key,omitempty"`
	SocketPath      string `json:"socket_path,omitempty"`
}

// AppImportSSHProbe is the body of the host key and connection test calls.
type AppImportSSHProbe struct {
	SSH           AppImportSSH `json:"ssh"`
	AllowPrivate  bool         `json:"allow_private,omitempty"`
	AllowLoopback bool         `json:"allow_loopback,omitempty"`
}

// AppImportSSHHostKey is one host key a server presented.
type AppImportSSHHostKey struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
}

// AppImportSSHOverview is what a connection test counted on the old host.
type AppImportSSHOverview struct {
	DockerVersion string `json:"docker_version"`
	OS            string `json:"os,omitempty"`
	Swarm         bool   `json:"swarm"`
	Containers    int    `json:"containers"`
	Running       int    `json:"running"`
	Volumes       int    `json:"volumes"`
	Images        int    `json:"images"`
}

// AppImportSSHResult is the answer of the host key and connection test calls.
type AppImportSSHResult struct {
	OK       bool                  `json:"ok"`
	Target   string                `json:"target,omitempty"`
	Kind     string                `json:"kind,omitempty"`
	Message  string                `json:"message,omitempty"`
	Reads    string                `json:"reads,omitempty"`
	Overview *AppImportSSHOverview `json:"overview,omitempty"`
	Keys     []AppImportSSHHostKey `json:"host_keys,omitempty"`
	Observed []AppImportSSHHostKey `json:"observed_host_keys,omitempty"`
	Warnings []string              `json:"warnings,omitempty"`
}

// SSHHostKeys calls POST /api/v1/migration/apps/ssh/host-keys.
func (c *Client) SSHHostKeys(ctx context.Context, req AppImportSSHProbe) (AppImportSSHResult, error) {
	var out AppImportSSHResult
	err := c.do(ctx, http.MethodPost, appImportPath+"/ssh/host-keys", req, &out)
	return out, err
}

// TestAppImportSSH calls POST /api/v1/migration/apps/ssh/test.
func (c *Client) TestAppImportSSH(ctx context.Context, req AppImportSSHProbe) (AppImportSSHResult, error) {
	var out AppImportSSHResult
	err := c.do(ctx, http.MethodPost, appImportPath+"/ssh/test", req, &out)
	return out, err
}

// AppImportVolumeCopyResult is one volume's copy state.
type AppImportVolumeCopyResult struct {
	SourceID      string   `json:"source_id"`
	App           string   `json:"app"`
	Volume        string   `json:"volume"`
	ContainerPath string   `json:"container_path"`
	State         string   `json:"state"`
	Files         int      `json:"files"`
	Bytes         int64    `json:"bytes"`
	Skipped       int      `json:"skipped_entries"`
	Warnings      []string `json:"warnings,omitempty"`
	Error         string   `json:"error,omitempty"`
	Fallback      string   `json:"fallback,omitempty"`
}

// AppImportVolumeCopy is the progress of a session's tunnel volume copy.
type AppImportVolumeCopy struct {
	Running  bool                        `json:"running"`
	MaxBytes int64                       `json:"max_bytes"`
	MaxFiles int                         `json:"max_files"`
	Results  []AppImportVolumeCopyResult `json:"results"`
}

func appImportVolumeCopyPath(id string) string {
	return appImportPath + "/sessions/" + url.PathEscape(id) + "/volumes/copy"
}

// StartAppImportVolumeCopy calls POST .../sessions/{id}/volumes/copy.
func (c *Client) StartAppImportVolumeCopy(ctx context.Context, id string, items []string) (AppImportVolumeCopy, error) {
	var out AppImportVolumeCopy
	err := c.do(ctx, http.MethodPost, appImportVolumeCopyPath(id), map[string][]string{"items": items}, &out)
	return out, err
}

// AppImportVolumeCopyStatus calls GET .../sessions/{id}/volumes/copy.
func (c *Client) AppImportVolumeCopyStatus(ctx context.Context, id string) (AppImportVolumeCopy, error) {
	var out AppImportVolumeCopy
	err := c.do(ctx, http.MethodGet, appImportVolumeCopyPath(id), nil, &out)
	return out, err
}
