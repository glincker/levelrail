package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// CapImagePull means the agent pulls an image into its own store when asked. An older
// agent never receives the request and the control plane skips the pre-pull.
const CapImagePull = "image_pull"

// ErrImagePullUnsupported is returned when the node runtime cannot pull on request.
var ErrImagePullUnsupported = errors.New("agent: this node's container runtime cannot pre-pull an image")

func executeImagePull(ctx context.Context, rt docker.Runtime, req *agentpb.PullImageRequest) error {
	p, ok := rt.(docker.ImagePuller)
	if !ok {
		return ErrImagePullUnsupported
	}
	var auth *docker.RegistryAuth
	if req.GetRegistryBearer() != "" {
		auth = &docker.RegistryAuth{Bearer: req.GetRegistryBearer()}
	}
	return p.PullImage(ctx, req.GetRef(), auth)
}

// PullImage asks the node to pull ref ahead of a move. Nothing is sent to an agent without the capability.
func (t *GRPCTransport) PullImage(ctx context.Context, ref string, auth *docker.RegistryAuth) error {
	if !t.caps.has(CapImagePull) {
		return ErrAgentLacksCapability
	}
	req := &agentpb.PullImageRequest{Ref: ref}
	if auth != nil {
		req.RegistryBearer = auth.Bearer
	}
	if _, err := t.mux.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_PullImage{PullImage: req}}); err != nil {
		return fmt.Errorf("agent: pull image: %w", err)
	}
	return nil
}
