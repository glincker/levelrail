package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// AgentClientHeader carries the MCP client name and version an agent-side
// server reports for the request. It is self-reported and informational.
const AgentClientHeader = "X-Agent-Client"

const (
	maxAgentNameLen        = 64
	maxAgentDescriptionLen = 500
	maxAgentClientLen      = 128
)

// agentIdentity labels a token as issued to an AI agent.
type agentIdentity struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func validateAgentIdentity(a *agentIdentity) (agentIdentity, error) {
	if a == nil {
		return agentIdentity{}, nil
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		return agentIdentity{}, errors.New("agent.name is required when agent is set")
	}
	if len(name) > maxAgentNameLen {
		return agentIdentity{}, fmt.Errorf("agent.name must be at most %d characters", maxAgentNameLen)
	}
	desc := strings.TrimSpace(a.Description)
	if len(desc) > maxAgentDescriptionLen {
		return agentIdentity{}, fmt.Errorf("agent.description must be at most %d characters", maxAgentDescriptionLen)
	}
	return agentIdentity{Name: name, Description: desc}, nil
}

type agentNameKey struct{}

func withAgentName(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, agentNameKey{}, name)
}

func agentNameFrom(ctx context.Context) string {
	name, _ := ctx.Value(agentNameKey{}).(string)
	return name
}

// sanitizeAgentClient drops control characters and caps the length of the
// self-reported client string before it is stored.
func sanitizeAgentClient(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > maxAgentClientLen {
		s = s[:maxAgentClientLen]
	}
	return s
}
