package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// CapFenceToken means the agent understands fence_token on destructive requests and refuses stale ones.
const CapFenceToken = "fence_token"

// staleFencePrefix starts the error string an agent returns for a refused stale request.
const staleFencePrefix = "stale_fence:"

var (
	// ErrStaleFence is returned when the agent refused a request whose token is below the highest it has seen.
	ErrStaleFence = errors.New("agent: stale fence token refused")
	// ErrFenceUnsupported is returned when agent.fence_required is on and the node's agent cannot enforce tokens.
	ErrFenceUnsupported = errors.New("agent: this node's agent does not support fence tokens and agent.fence_required is on; update the agent or turn the setting off")
	// ErrStaleAction is returned when an action finished after its target had moved on to a newer token.
	ErrStaleAction = errors.New("agent: stale_action_rejected: the placement changed while the action ran")
)

type fenceCtxKey struct{}

type fenceInfo struct {
	target string
	token  uint64
}

// WithFence marks destructive calls made with the returned context as acting for target at token.
func WithFence(ctx context.Context, target string, token uint64) context.Context {
	return context.WithValue(ctx, fenceCtxKey{}, fenceInfo{target: target, token: token})
}

// FenceCtx stamps ctx with the token read for target now. Call it after any placement change
// so a stale in-flight action keeps the older token. A read failure or a zero token leaves ctx unfenced.
func FenceCtx(ctx context.Context, read func(context.Context, string) (uint64, error), target string) context.Context {
	if read == nil || target == "" {
		return ctx
	}
	tok, err := read(ctx, target)
	if err != nil || tok == 0 {
		return ctx
	}
	return WithFence(ctx, target, tok)
}

func fenceFrom(ctx context.Context) (fenceInfo, bool) {
	fi, ok := ctx.Value(fenceCtxKey{}).(fenceInfo)
	return fi, ok && fi.target != "" && fi.token > 0
}

// FenceHooks connect the transport to the control plane's records. Nil fields mean no enforcement.
type FenceHooks struct {
	// Required reports the live agent.fence_required setting.
	Required func(ctx context.Context) bool
	// Current returns the recorded token of target.
	Current func(ctx context.Context, target string) (uint64, error)
	// Rejected records a stale_action_rejected event.
	Rejected func(ctx context.Context, nodeID, target string, sent, current uint64, op string)
}

// WithFenceHooks installs the hooks used by every node transport of the server.
func WithFenceHooks(h FenceHooks) Option {
	return func(s *Server) { s.fence = h }
}

// fenceStamp picks the token and target to put on a destructive request, or refuses it for an old agent.
func (t *GRPCTransport) fenceStamp(ctx context.Context) (uint64, string, error) {
	if !t.caps.has(CapFenceToken) {
		if t.fence.Required != nil && t.fence.Required(ctx) {
			return 0, "", ErrFenceUnsupported
		}
		return 0, "", nil
	}
	fi, ok := fenceFrom(ctx)
	if !ok {
		return 0, "", nil
	}
	return fi.token, fi.target, nil
}

// destructive sends a request built with the current token and rejects a result that finished after a newer token.
func (t *GRPCTransport) destructive(ctx context.Context, op string, build func(token uint64, target string) *agentpb.AgentRequest) (*agentpb.AgentResponse, error) {
	token, target, err := t.fenceStamp(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := t.mux.Call(ctx, build(token, target))
	if err != nil {
		if strings.Contains(err.Error(), staleFencePrefix) {
			return nil, fmt.Errorf("%w: %s", ErrStaleFence, err.Error())
		}
		return nil, err
	}
	if token == 0 || t.fence.Current == nil {
		return resp, nil
	}
	cur, cerr := t.fence.Current(ctx, target)
	if cerr == nil && cur > token {
		if t.fence.Rejected != nil {
			t.fence.Rejected(ctx, t.nodeID, target, token, cur, op)
		}
		return nil, ErrStaleAction
	}
	return resp, nil
}

// FenceBook is the agent's memory of the highest token seen per target.
type FenceBook struct {
	mu   sync.Mutex
	seen map[string]uint64
	path string
}

// NewFenceBook returns an in-memory book. Without SetPath a restarted agent forgets its tokens.
func NewFenceBook() *FenceBook { return &FenceBook{seen: map[string]uint64{}} }

// SetPath loads and keeps the book in a file so a restart does not reopen the window for stale requests.
func (b *FenceBook) SetPath(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.path = path
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from the agent data dir
	if err != nil {
		return
	}
	var m map[string]uint64
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	for k, v := range m {
		if v > b.seen[k] {
			b.seen[k] = v
		}
	}
}

// Admit accepts a token equal to or above the highest seen for target and refuses a lower one. Token 0 is unfenced.
func (b *FenceBook) Admit(target string, token uint64) error {
	if token == 0 || target == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	hi := b.seen[target]
	if token < hi {
		return fmt.Errorf("%s token %d for %s is below %d", staleFencePrefix, token, target, hi)
	}
	if token > hi {
		b.seen[target] = token
		b.saveLocked()
	}
	return nil
}

func (b *FenceBook) saveLocked() {
	if b.path == "" {
		return
	}
	raw, err := json.Marshal(b.seen)
	if err != nil {
		return
	}
	tmp := b.path + ".tmp"
	if os.MkdirAll(filepath.Dir(b.path), 0o700) != nil || os.WriteFile(tmp, raw, 0o600) != nil {
		return
	}
	_ = os.Rename(tmp, b.path)
}

// CheckRequest applies Admit to the fence fields of a destructive request; other requests pass.
func (b *FenceBook) CheckRequest(req *agentpb.AgentRequest) error {
	var token uint64
	var target string
	switch op := req.GetOp().(type) {
	case *agentpb.AgentRequest_Start:
		token, target = op.Start.GetFenceToken(), op.Start.GetFenceTarget()
	case *agentpb.AgentRequest_Stop:
		token, target = op.Stop.GetFenceToken(), op.Stop.GetFenceTarget()
	case *agentpb.AgentRequest_Remove:
		token, target = op.Remove.GetFenceToken(), op.Remove.GetFenceTarget()
	case *agentpb.AgentRequest_RemoveNetwork:
		token, target = op.RemoveNetwork.GetFenceToken(), op.RemoveNetwork.GetFenceTarget()
	case *agentpb.AgentRequest_NetworkDisconnect:
		token, target = op.NetworkDisconnect.GetFenceToken(), op.NetworkDisconnect.GetFenceTarget()
	case *agentpb.AgentRequest_RemoveVolume:
		token, target = op.RemoveVolume.GetFenceToken(), op.RemoveVolume.GetFenceTarget()
	case *agentpb.AgentRequest_Cleanup:
		token, target = op.Cleanup.GetFenceToken(), op.Cleanup.GetFenceTarget()
	default:
		return nil
	}
	return b.Admit(target, token)
}

// processFence outlives reconnects so a stale request cannot slip in right after a session restarts.
var processFence = NewFenceBook()

// WithFenceState keeps the agent's highest seen tokens in path.
func WithFenceState(path string) SessionOption {
	return func(*sessionConfig) { processFence.SetPath(path) }
}
