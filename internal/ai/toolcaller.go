package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolCaller runs tool calls against the platform's own MCP tool
// surface, connected in-process: no subprocess, no network transport.
// It builds the same *mcp.Server internal/mcptools.NewServer gives
// cmd/levelrail-mcp and connects an MCP client to it over
// mcp.NewInMemoryTransports, the SDK's own in-process test harness,
// reused here as a real (not test-only) transport.
type ToolCaller struct {
	session *mcp.ClientSession
}

// NewToolCaller builds a ToolCaller whose tools call back into the
// control plane's own REST API through client, exactly like an external
// MCP client (or cmd/levelrail-cli) would.
func NewToolCaller(ctx context.Context, client *apiclient.Client) (*ToolCaller, error) {
	server := mcptools.NewServer(client)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Deliberately context.Background(), not ctx: the server goroutine
	// must outlive this constructor call for as long as the session is
	// open, ending on session.Close() (Close below), not on ctx's own
	// cancellation.
	go func() { _ = server.Run(context.Background(), serverTransport) }() //nolint:gosec // see comment above

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "levelrail-ai-assistant", Version: "1"}, nil)
	session, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("ai: connect in-process mcp client: %w", err)
	}
	return &ToolCaller{session: session}, nil
}

// Close ends the underlying MCP session.
func (tc *ToolCaller) Close() error {
	return tc.session.Close()
}

// ListTools returns every tool the platform exposes, translated to the
// provider-agnostic ToolSpec shape.
func (tc *ToolCaller) ListTools(ctx context.Context) ([]ToolSpec, error) {
	var out []ToolSpec
	cursor := ""
	for {
		res, err := tc.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("ai: list tools: %w", err)
		}
		for _, t := range res.Tools {
			schema, err := json.Marshal(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("ai: marshal input schema for tool %q: %w", t.Name, err)
			}
			out = append(out, ToolSpec{Name: t.Name, Description: t.Description, InputSchema: schema, Traits: TraitsFromMCP(t.Annotations, t.Meta)})
		}
		if res.NextCursor == "" {
			break
		}
		cursor = res.NextCursor
	}
	return out, nil
}

// Call invokes name with args (a JSON object), returning its result as
// text (every real tool here returns JSON structured content, joined
// from TextContent blocks the same way cmd/levelrail-mcp's own tests
// already extract a result) and whether the call itself errored.
func (tc *ToolCaller) Call(ctx context.Context, name string, args json.RawMessage) (result string, isError bool, err error) {
	var arguments any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return "", false, fmt.Errorf("ai: unmarshal arguments for tool %q: %w", name, err)
		}
	}

	res, err := tc.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return "", false, fmt.Errorf("ai: call tool %q: %w", name, err)
	}

	var sb strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(text.Text)
		}
	}
	if sb.Len() == 0 && res.StructuredContent != nil {
		structured, err := json.Marshal(res.StructuredContent)
		if err == nil {
			sb.Write(structured)
		}
	}
	return sb.String(), res.IsError, nil
}
