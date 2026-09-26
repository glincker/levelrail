package pipeline

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the persistence surface the Engine needs. *store.DB satisfies it.
type Store interface {
	GetPipeline(ctx context.Context, id string) (store.Pipeline, error)
	ListPipelines(ctx context.Context, app string) ([]store.Pipeline, error)
	CreatePipelineRun(ctx context.Context, r store.PipelineRun) (store.PipelineRun, error)
	GetPipelineRun(ctx context.Context, id string) (store.PipelineRun, error)
	ListActivePipelineRuns(ctx context.Context) ([]store.PipelineRun, error)
	ListPipelineRunsInGroup(ctx context.Context, group string) ([]store.PipelineRun, error)
	SetPipelineRunStatus(ctx context.Context, id, status, reason string, started, finished *time.Time) error
	RequestPipelineRunCancel(ctx context.Context, id string) error
	CreatePipelineJobs(ctx context.Context, jobs []store.PipelineJob) error
	ListPipelineJobs(ctx context.Context, runID string) ([]store.PipelineJob, error)
	SetPipelineJobStatus(ctx context.Context, id, status, reason, nodeID string, attempt int, started, finished *time.Time) error
	SetPipelineJobOutputs(ctx context.Context, id, outputsJSON string) error
	SetPipelineStepStatus(ctx context.Context, jobID string, idx int, status, reason string, exitCode *int, attempt int, started, finished *time.Time) error
	AppendPipelineLogs(ctx context.Context, lines []store.PipelineLogLine) error
	CreatePipelineApproval(ctx context.Context, a store.PipelineApproval) error
	ListPipelineApprovals(ctx context.Context, runID string) ([]store.PipelineApproval, error)
	PrunePipelineRuns(ctx context.Context, keep int) (int64, error)
	AddPipelineTriggerLog(ctx context.Context, e store.PipelineTriggerLog) error
	DecidePipelineRunHold(ctx context.Context, id string, approved bool, by string, now time.Time) (bool, error)
}

// Runtime is the slice of docker.Runtime a job needs, so tests can fake it
// and remote nodes work through the same interface as the local daemon.
type Runtime interface {
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, id string) error
	Remove(ctx context.Context, id string, force bool) error
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
	EnsureVolume(ctx context.Context, name string) error
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}

// VolumeRemover is an optional Runtime capability used for best-effort
// workspace cleanup; runtimes without it leave volumes for the volume prune.
type VolumeRemover interface {
	RemoveVolume(ctx context.Context, name string) error
}

// RuntimeResolver returns the runtime for a node ID ("" means the local node).
type RuntimeResolver func(nodeID string) (Runtime, error)

// SecretResolver reads an app's secret value. *secrets.Manager satisfies it.
type SecretResolver interface {
	Resolve(ctx context.Context, app, key string) (string, error)
}

// ErrNoRepo is returned by Source when an app has no connected repository.
var ErrNoRepo = errors.New("pipeline: app has no connected repository")

// Source resolves the repository a job checks out.
type Source interface {
	RepoInfo(ctx context.Context, app string) (url, token string, err error)
}

// BuildRequest is one image build for the build step.
type BuildRequest struct {
	App        string
	Ref        string
	SHA        string
	Type       string
	Context    string
	Dockerfile string
	Image      string
	Tag        string
}

// DeployRequest is one deploy of an existing service to an image.
type DeployRequest struct {
	Service  string
	Image    string
	Strategy string
	Wait     time.Duration
}

// Actions are the control-plane primitives non-container steps call. They
// wrap the existing build and deploy code; the Engine never reconciles.
type Actions interface {
	Build(ctx context.Context, req BuildRequest, log func(line string)) (image string, err error)
	Deploy(ctx context.Context, req DeployRequest, log func(line string)) error
	Promote(ctx context.Context, from, to string, log func(line string)) (image string, err error)
	Rollback(ctx context.Context, service string, log func(line string)) (image string, err error)
	Notify(ctx context.Context, app string, succeeded bool, message string) error
}

// Config wires an Engine. Zero limits fall back to the defaults below;
// cmd wiring overrides them from environment variables.
type Config struct {
	Store   Store
	Runtime RuntimeResolver
	Actions Actions
	Secrets SecretResolver
	Source  Source
	// RunEnv returns extra environment variables for a run's steps, for
	// example the preview environment a pull request run belongs to. Optional.
	RunEnv func(ctx context.Context, run store.PipelineRun) map[string]string
	Logger *slog.Logger
	Now    func() time.Time
	NewID  func() string

	// NamePrefix namespaces containers and volumes (the brand short name).
	NamePrefix string
	// GitImage runs the checkout; it needs a POSIX shell and git.
	GitImage        string
	MaxParallelJobs int
	MaxLogLines     int
	StepTimeout     time.Duration
	JobTimeout      time.Duration
	ApprovalTimeout time.Duration
	KeepRuns        int
}

const (
	defaultGitImage        = "alpine/git"
	defaultMaxParallelJobs = 4
	defaultMaxLogLines     = 50000
	defaultStepTimeout     = 30 * time.Minute
	defaultJobTimeout      = time.Hour
	defaultApprovalTimeout = 72 * time.Hour
	defaultKeepRuns        = 100
)

func (c *Config) applyDefaults() {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.NewID == nil {
		c.NewID = randomID
	}
	if c.NamePrefix == "" {
		c.NamePrefix = "pl"
	}
	if c.GitImage == "" {
		c.GitImage = defaultGitImage
	}
	if c.MaxParallelJobs <= 0 {
		c.MaxParallelJobs = defaultMaxParallelJobs
	}
	if c.MaxLogLines <= 0 {
		c.MaxLogLines = defaultMaxLogLines
	}
	if c.StepTimeout <= 0 {
		c.StepTimeout = defaultStepTimeout
	}
	if c.JobTimeout <= 0 {
		c.JobTimeout = defaultJobTimeout
	}
	if c.ApprovalTimeout <= 0 {
		c.ApprovalTimeout = defaultApprovalTimeout
	}
	if c.KeepRuns <= 0 {
		c.KeepRuns = defaultKeepRuns
	}
}
