package buildkitd

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings/registrymirror"
)

const (
	// ManagedLabel marks the containers and volumes this package owns.
	ManagedLabel = "levelrail.managed"
	// RoleLabel names what a managed container is for.
	RoleLabel = "levelrail.role"
	roleValue = "buildkitd"

	rootlessDataPath = "/home/user/.local/share/buildkit"
	rootfulDataPath  = "/var/lib/buildkit"
	configDir        = "/etc/buildkit"
)

var (
	// ErrConflict is what an Engine returns when a create races another creator.
	ErrConflict = errors.New("buildkitd: container name already in use")
	// ErrForeign means an object with our name exists but is not ours or is not as expected.
	ErrForeign = errors.New("buildkitd: existing object is not the managed builder")
)

// rootlessSecurityOpt is what the rootless image needs instead of Privileged.
var rootlessSecurityOpt = []string{"seccomp=unconfined", "apparmor=unconfined", "systempaths=unconfined"}

// Mount is one observed container mount.
type Mount struct {
	Name        string
	Destination string
}

// Observed is the trimmed inspect result the manager validates.
type Observed struct {
	ID         string
	Image      string
	Running    bool
	Privileged bool
	Labels     map[string]string
	Mounts     []Mount
}

// CreateSpec is the desired managed container.
type CreateSpec struct {
	Name        string
	Image       string
	Cmd         []string
	Labels      map[string]string
	SecurityOpt []string
	PidsLimit   int64
	CPUShares   int64
	Volume      string
	DataPath    string
}

// Engine is the engine API surface the API-only manager uses, so it works
// against a remote or VM hosted engine where no host socket can be shared.
type Engine interface {
	// InspectContainer returns nil, nil when the container does not exist.
	InspectContainer(ctx context.Context, name string) (*Observed, error)
	CreateContainer(ctx context.Context, spec CreateSpec) (string, error)
	StartContainer(ctx context.Context, id string) error
	// PutFiles extracts a tar stream into dir inside a created container.
	PutFiles(ctx context.Context, id, dir string, body io.Reader) error
	HasImage(ctx context.Context, ref string) (bool, error)
	PullImage(ctx context.Context, ref string) error
	// InspectVolume reports whether the volume exists and its labels.
	InspectVolume(ctx context.Context, name string) (labels map[string]string, found bool, err error)
	CreateVolume(ctx context.Context, name string, labels map[string]string) error
}

// APIManager keeps the managed builder container present and running using
// engine API calls only. Concurrent callers are serialized.
type APIManager struct {
	cfg Config
	eng Engine
	mu  sync.Mutex
}

// NewAPIManager refuses an image that is not pinned by sha256 digest.
func NewAPIManager(cfg Config, eng Engine) (*APIManager, error) {
	if err := CheckPinned(cfg.Image); err != nil {
		return nil, err
	}
	return &APIManager{cfg: cfg, eng: eng}, nil
}

// IsRootlessImage reports whether ref names the rootless BuildKit variant.
func IsRootlessImage(ref string) bool { return strings.Contains(ref, "-rootless") }

func (m *APIManager) imageRef(ctx context.Context) string {
	if m.cfg.MirrorPrefix == nil {
		return m.cfg.Image
	}
	return registrymirror.Apply(m.cfg.Image, m.cfg.MirrorPrefix(ctx))
}

func (m *APIManager) dataPath(ref string) string {
	if IsRootlessImage(ref) {
		return rootlessDataPath
	}
	return rootfulDataPath
}

// Ensure leaves exactly one validated, running managed builder.
func (m *APIManager) Ensure(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref := m.imageRef(ctx)
	if err := m.ensureVolume(ctx); err != nil {
		return err
	}
	obs, err := m.eng.InspectContainer(ctx, ContainerName)
	if err != nil {
		return fmt.Errorf("buildkitd: inspect: %w", err)
	}
	if obs == nil {
		if obs, err = m.create(ctx, ref); err != nil {
			return err
		}
	}
	if err := m.validate(obs, ref); err != nil {
		return err
	}
	if obs.Running {
		return nil
	}
	if err := m.eng.StartContainer(ctx, obs.ID); err != nil {
		return fmt.Errorf("buildkitd: start: %w", err)
	}
	return nil
}

func (m *APIManager) ensureVolume(ctx context.Context) error {
	labels, found, err := m.eng.InspectVolume(ctx, VolumeName)
	if err != nil {
		return fmt.Errorf("buildkitd: inspect volume: %w", err)
	}
	if found {
		if labels[ManagedLabel] != "true" {
			return fmt.Errorf("%w: volume %s has no managed label", ErrForeign, VolumeName)
		}
		return nil
	}
	if err := m.eng.CreateVolume(ctx, VolumeName, managedLabels()); err != nil {
		return fmt.Errorf("buildkitd: create volume: %w", err)
	}
	return nil
}

// create builds the container, or on a lost race returns the winner's state.
func (m *APIManager) create(ctx context.Context, ref string) (*Observed, error) {
	if ok, err := m.eng.HasImage(ctx, ref); err != nil {
		return nil, fmt.Errorf("buildkitd: inspect image: %w", err)
	} else if !ok {
		if err := m.eng.PullImage(ctx, ref); err != nil {
			return nil, fmt.Errorf("buildkitd: pull %s: %w", ref, err)
		}
	}
	secOpt := m.cfg.SecurityOpt
	if len(secOpt) == 0 {
		secOpt = rootlessSecurityOpt
	}
	spec := CreateSpec{
		Name: ContainerName, Image: ref, Labels: managedLabels(),
		SecurityOpt: append([]string(nil), secOpt...),
		Cmd:         []string{"--oci-worker-no-process-sandbox", "--config", configDir + "/buildkitd.toml"},
		Volume:      VolumeName, DataPath: m.dataPath(ref),
		CPUShares: m.cfg.CPUShares,
	}
	if m.cfg.PidsLimit != nil {
		spec.PidsLimit = int64(max(m.cfg.PidsLimit(ctx), 0))
	}
	id, err := m.eng.CreateContainer(ctx, spec)
	if errors.Is(err, ErrConflict) {
		obs, ierr := m.eng.InspectContainer(ctx, ContainerName)
		if ierr != nil || obs == nil {
			return nil, fmt.Errorf("buildkitd: create conflict, re-inspect: %w", errors.Join(ierr, ErrConflict))
		}
		return obs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("buildkitd: create: %w", err)
	}
	if err := m.eng.PutFiles(ctx, id, "/etc", tomlTar(m.cfg.TOMLBody(0))); err != nil {
		return nil, fmt.Errorf("buildkitd: write config: %w", err)
	}
	return &Observed{ID: id, Image: ref, Labels: managedLabels(),
		Mounts: []Mount{{Name: VolumeName, Destination: spec.DataPath}}}, nil
}

// validate never trusts a container by name alone.
func (m *APIManager) validate(o *Observed, ref string) error {
	switch {
	case o.Labels[ManagedLabel] != "true" || o.Labels[RoleLabel] != roleValue:
		return fmt.Errorf("%w: %s lacks the managed labels", ErrForeign, ContainerName)
	case o.Privileged:
		return fmt.Errorf("%w: %s is privileged", ErrForeign, ContainerName)
	case !sameImage(o.Image, ref):
		return fmt.Errorf("%w: %s runs %s, expected %s", ErrForeign, ContainerName, o.Image, ref)
	}
	for _, mt := range o.Mounts {
		if mt.Name == VolumeName && mt.Destination == m.dataPath(ref) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s does not mount %s", ErrForeign, ContainerName, VolumeName)
}

// sameImage compares by digest when want is pinned, because Podman reports
// the image without its tag. An unpinned want only tolerates a missing tag.
func sameImage(got, want string) bool {
	if got == want {
		return true
	}
	if i := strings.Index(want, sha256Prefix); i >= 0 {
		return strings.HasSuffix(got, want[i:])
	}
	return repoOf(got) == repoOf(want) && !strings.Contains(got, sha256Prefix)
}

// repoOf drops a trailing :tag, leaving a registry port untouched.
func repoOf(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i]
	}
	return ref
}

func managedLabels() map[string]string {
	return map[string]string{ManagedLabel: "true", RoleLabel: roleValue}
}

func tomlTar(body string) io.Reader {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	_ = tw.WriteHeader(&tar.Header{Name: "buildkit/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: time.Unix(0, 0)})
	_ = tw.WriteHeader(&tar.Header{Name: "buildkit/buildkitd.toml", Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(0, 0)})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	return &b
}

// DecodePullStream returns the first error carried by an image pull JSON stream.
func DecodePullStream(r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		var msg struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if msg.ErrorDetail.Message != "" {
			return errors.New(msg.ErrorDetail.Message)
		}
		if msg.Error != "" {
			return errors.New(msg.Error)
		}
	}
}
