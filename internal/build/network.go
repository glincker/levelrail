package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/moby/buildkit/solver/pb"
)

// EnvBuildNetwork selects the network policy for RUN steps of a build.
const EnvBuildNetwork = "APP_BUILD_NETWORK"

// KeyBuildNetwork is the runtime setting key.
const KeyBuildNetwork = "security.build_network"

// NetworkMode is how much network a build's RUN steps get.
type NetworkMode string

const (
	// NetworkOpen leaves RUN steps with whatever the engine's BuildKit gives
	// them. It is the default and the behaviour before the knob existed.
	NetworkOpen NetworkMode = "open"
	// NetworkNone runs every RUN step without a network interface.
	NetworkNone NetworkMode = "none"
	// NetworkBlockPrivate is reserved for blocking metadata and private
	// ranges while keeping the internet. No build path can enforce it yet.
	NetworkBlockPrivate NetworkMode = "block-private"
)

// ErrNetworkUnenforceable fails a build instead of running it with a policy
// that would silently not apply.
var ErrNetworkUnenforceable = errors.New("build: APP_BUILD_NETWORK=block-private cannot be enforced on this build path yet; use open or none")

// ParseNetworkMode reads APP_BUILD_NETWORK. Empty is open; an unknown value
// is an error so a typo on a security setting never degrades to open.
func ParseNetworkMode(raw string) (NetworkMode, error) {
	switch m := NetworkMode(strings.ToLower(strings.TrimSpace(raw))); m {
	case "", NetworkOpen:
		return NetworkOpen, nil
	case NetworkNone, NetworkBlockPrivate:
		return m, nil
	default:
		return "", fmt.Errorf("build: unknown %s %q (want open, none or block-private)", EnvBuildNetwork, raw)
	}
}

// forceNetworkAttr returns the dockerfile frontend attribute for the mode.
func (m NetworkMode) forceNetworkAttr() (string, bool, error) {
	switch m {
	case NetworkNone:
		return "none", true, nil
	case NetworkBlockPrivate:
		return "", false, ErrNetworkUnenforceable
	default:
		return "", false, nil
	}
}

// ErrNetworkFrontend refuses a custom Dockerfile frontend: force-network-mode
// is only a hint to the stock frontend, a foreign one can return RUN steps
// with the default network.
var ErrNetworkFrontend = errors.New("build: a custom Dockerfile frontend (# syntax= or BUILDKIT_SYNTAX) is refused while security.build_network is not open; remove it or use the stock docker/dockerfile frontend")

// checkFrontend allows only the stock frontend, whose image honours the
// force-network-mode attribute, when the mode restricts the network.
func (m NetworkMode) checkFrontend(dockerfile string, args map[string]string) error {
	if ref, ok := args["BUILDKIT_SYNTAX"]; ok && !stockFrontend(ref) {
		return ErrNetworkFrontend
	}
	dt, err := os.ReadFile(dockerfile) //nolint:gosec // path already confined to the build root by the caller
	if err != nil {
		return fmt.Errorf("build: read dockerfile: %w", err)
	}
	if ref, _, _, ok := parser.DetectSyntax(dt); ok && !stockFrontend(ref) {
		return ErrNetworkFrontend
	}
	return nil
}

// stockFrontend reports whether ref is the docker/dockerfile image on Docker Hub.
func stockFrontend(ref string) bool {
	ref = strings.TrimSpace(ref)
	for _, p := range []string{"docker.io/", "index.docker.io/"} {
		ref = strings.TrimPrefix(ref, p)
	}
	if i := strings.IndexAny(ref, ":@"); i >= 0 {
		ref = ref[:i]
	}
	return ref == "docker/dockerfile"
}

// applyRailpack rewrites every exec op of def to the mode's network.
func (m NetworkMode) applyRailpack(def *llb.Definition) (*llb.Definition, error) {
	switch m {
	case NetworkNone:
		return rewriteExecs(def, func(ex *pb.ExecOp) { ex.Network = pb.NetMode_NONE })
	case NetworkBlockPrivate:
		return nil, ErrNetworkUnenforceable
	default:
		return def, nil
	}
}

// NetworkFacts states what a build's RUN steps can reach under mode. The
// open wording names what the code does not restrict, not what was probed.
type NetworkFacts struct {
	Mode       NetworkMode
	Enforced   bool
	Summary    string
	NotProven  string
	DocsAnchor string
}

// Facts describes the effective policy for the doctor and docs.
func (m NetworkMode) Facts() NetworkFacts {
	f := NetworkFacts{Mode: m, DocsAnchor: "/build-center#build-network"}
	switch m {
	case NetworkNone:
		f.Enforced = true
		f.Summary = "RUN steps have no network. Base image pulls and ADD of a URL are done by BuildKit outside RUN and still reach the network."
		f.NotProven = "Not yet confirmed on a real host."
	case NetworkBlockPrivate:
		f.Summary = "block-private is set but no build path can enforce it; builds fail until it is open or none."
	default:
		f.Summary = "RUN steps use the engine's default build network. Levelrail sets no restriction, so cloud metadata, the Docker gateway and the LAN are reachable if the engine's default network can route to them."
		f.NotProven = "Reachability of those ranges has not been probed on a real host."
	}
	return f
}

// networkResolver reads the saved platform setting on every build so a change
// applies without a restart. Nil means no settings store (agents): env only.
var networkResolver atomic.Pointer[func(context.Context) string]

// SetNetworkResolver wires the platform settings value in at boot.
func SetNetworkResolver(fn func(context.Context) string) {
	if fn == nil {
		networkResolver.Store(nil)
		return
	}
	networkResolver.Store(&fn)
}

func resolveNetwork(ctx context.Context) (NetworkMode, error) {
	if fn := networkResolver.Load(); fn != nil {
		return ParseNetworkMode((*fn)(ctx))
	}
	return ParseNetworkMode(os.Getenv(EnvBuildNetwork))
}
