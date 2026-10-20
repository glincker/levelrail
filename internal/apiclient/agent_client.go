package apiclient

import "context"

// AgentClientHeader carries the calling MCP client's name and version to the
// control plane so audit entries can record it.
const AgentClientHeader = "X-Agent-Client"

type agentClientKey struct{}

// WithAgentClient returns a context whose requests report the MCP client
// name and version in AgentClientHeader.
func WithAgentClient(ctx context.Context, name, version string) context.Context {
	if name == "" {
		return ctx
	}
	label := name
	if version != "" {
		label += "/" + version
	}
	return context.WithValue(ctx, agentClientKey{}, label)
}

func agentClientFrom(ctx context.Context) string {
	label, _ := ctx.Value(agentClientKey{}).(string)
	return label
}
