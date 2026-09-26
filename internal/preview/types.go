// Package preview captures a thumbnail of each deployed release with a
// short-lived browser container and keeps them within bounded storage.
package preview

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Record statuses.
const (
	StatusOK      = "ok"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Skip and failure reason codes stored on a Record.
const (
	ReasonLowRAM        = "low_ram"
	ReasonLowDisk       = "low_disk"
	ReasonImageUnusable = "browser_image_unavailable"
	ReasonNoAppNetwork  = "no_app_network"
	ReasonRemoteNode    = "remote_node"
	ReasonUnreachable   = "unreachable"
	ReasonHTTPStatus    = "http_status"
	ReasonAuthWall      = "auth_wall"
	ReasonBlankImage    = "blank_image"
	ReasonBadImage      = "bad_image"
	ReasonTimeout       = "timeout"
	ReasonCaptureFailed = "capture_failed"
)

// ErrNotFound means no preview exists for the requested deployment.
var ErrNotFound = errors.New("preview: not found")

// ErrDisabled means previews are switched off, globally or for the app.
var ErrDisabled = errors.New("preview: disabled")

// ErrInvalid wraps every settings validation failure.
var ErrInvalid = errors.New("preview: invalid setting")

// Record is one deployment's preview outcome. Only StatusOK rows own a file.
type Record struct {
	DeploymentID string
	App          string
	Path         string
	Bytes        int64
	Width        int
	Height       int
	Status       string
	Reason       string
	Detail       string
	HTTPStatus   int
	CapturedAt   time.Time
	LastViewedAt time.Time
}

// LastUsed is the recency LRU eviction orders by.
func (r Record) LastUsed() time.Time {
	if r.LastViewedAt.After(r.CapturedAt) {
		return r.LastViewedAt
	}
	return r.CapturedAt
}

// AppSettings is one app's opt-in and capture options.
type AppSettings struct {
	App     string
	Enabled bool
	Path    string
	WaitMS  int
}

// Bounds on the per-app capture options.
const (
	DefaultPath = "/"
	MaxPathLen  = 512
	MaxWaitMS   = 10000
)

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._~!$&'()*+,;=:@%/?#\[\]-]*$`)

// ValidatePath rejects anything that is not an absolute path on the app itself.
func ValidatePath(p string) error {
	switch {
	case p == "" || len(p) > MaxPathLen:
		return fmt.Errorf("%w: path must be 1 to %d characters", ErrInvalid, MaxPathLen)
	case len(p) > 1 && p[1] == '/':
		return fmt.Errorf("%w: path must not start with //", ErrInvalid)
	case !pathPattern.MatchString(p):
		return fmt.Errorf("%w: path must be an absolute path such as /pricing", ErrInvalid)
	}
	return nil
}

// Validate checks settings before they are stored.
func (s AppSettings) Validate() error {
	if err := ValidatePath(s.Path); err != nil {
		return err
	}
	if s.WaitMS < 0 || s.WaitMS > MaxWaitMS {
		return fmt.Errorf("%w: wait_ms must be between 0 and %d", ErrInvalid, MaxWaitMS)
	}
	return nil
}

// SettingsPatch updates only the fields that are non-nil.
type SettingsPatch struct {
	Enabled *bool
	Path    *string
	WaitMS  *int
}

// Target is where to point the browser for an app's current release.
type Target struct {
	DeploymentID string
	Image        string
	Network      string
	Host         string
	Port         int
}

// SkipError tells the Manager a capture must not run, recording Reason
// against DeploymentID (empty means nothing is recorded).
type SkipError struct {
	DeploymentID string
	Reason       string
	Detail       string
}

func (e *SkipError) Error() string {
	return fmt.Sprintf("preview: skipped (%s) %s", e.Reason, e.Detail)
}

// Resolver maps an app to its live release and Docker network.
type Resolver interface {
	// Resolve returns the capture target for app's release currently
	// serving image ("" means whatever is current), or a *SkipError.
	Resolve(ctx context.Context, app, image string) (Target, error)
	// CurrentDeployment returns the deployment serving production, or "".
	CurrentDeployment(ctx context.Context, app string) (string, error)
	AppExists(ctx context.Context, app string) (bool, error)
}

// Store persists settings, records and the small image-tracking state.
type Store interface {
	GetPreviewSettings(ctx context.Context, app string) (AppSettings, error)
	SavePreviewSettings(ctx context.Context, s AppSettings) error
	UpsertPreviewRecord(ctx context.Context, r Record) error
	GetPreviewRecord(ctx context.Context, deploymentID string) (*Record, error)
	ListPreviewRecords(ctx context.Context, app string) ([]Record, error)
	ListAllPreviewRecords(ctx context.Context) ([]Record, error)
	DeletePreviewRecords(ctx context.Context, deploymentIDs []string) error
	TouchPreviewViewed(ctx context.Context, deploymentID string, at time.Time) error
	GetPreviewState(ctx context.Context, key string) (string, error)
	SetPreviewState(ctx context.Context, key, value string) error
}

// Browser is a running capture browser.
type Browser interface {
	// Endpoint is the host address of the browser's debugging port.
	Endpoint() string
	Close()
}

// Runner is the Docker surface previews use.
type Runner interface {
	EnsureImageID(ctx context.Context, ref string) (id string, pulled bool, err error)
	StartBrowser(ctx context.Context, spec docker.BrowserSpec) (Browser, error)
	ListContainersByLabel(ctx context.Context, label string) ([]docker.LabeledContainer, error)
	Remove(ctx context.Context, id string, force bool) error
	ImageInUse(ctx context.Context, imageID string) (bool, error)
	RemoveImageByID(ctx context.Context, imageID string) error
}

// dockerRunner adapts *docker.Client to Runner.
type dockerRunner struct{ *docker.Client }

func (r dockerRunner) StartBrowser(ctx context.Context, spec docker.BrowserSpec) (Browser, error) {
	b, err := r.Client.StartBrowser(ctx, spec)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// NewDockerRunner returns a Runner backed by the real Docker client.
func NewDockerRunner(c *docker.Client) Runner { return dockerRunner{c} }
