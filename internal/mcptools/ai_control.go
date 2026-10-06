package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	aiControlStatusTool = "ai_control_status"
	aiControlTimeout    = 3 * time.Second
	aiModeOff           = "off"
	aiModeObserve       = "observe"
	aiDisabledMessage   = "agent access is disabled"
)

type aiControlStatusOutput struct {
	Mode            string   `json:"mode"`
	AllowedEnvKinds []string `json:"allowed_env_kinds,omitempty"`
	Explanation     string   `json:"explanation"`
}

func registerAIControlTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        aiControlStatusTool,
		Description: "Show the AI control mode an administrator set for agents on this instance (off, observe, operate, admin) and what it allows. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, aiControlStatusOutput, error) {
		mode, kinds, _, err := fetchAIControl(ctx, client)
		if err != nil {
			return nil, aiControlStatusOutput{}, fmt.Errorf("get ai control: %w", err)
		}
		return nil, aiControlStatusOutput{Mode: mode, AllowedEnvKinds: kinds, Explanation: aiControlExplanation(mode)}, nil
	})
}

func aiControlExplanation(mode string) string {
	switch mode {
	case aiModeOff:
		return "An administrator turned agent access off. Every request is refused until they change it."
	case aiModeObserve:
		return "Agents may only read."
	case "operate":
		return "Agents may read, write and deploy in allowed environment kinds, never with root."
	default:
		return "Agents may use any ability their token holds in allowed environment kinds. A human must approve protected deploys."
	}
}

// fetchAIControl reads the mode and whether this token is an agent. The disabled-agents 403 means an agent and off.
func fetchAIControl(ctx context.Context, client *apiclient.Client) (mode string, kinds []string, callerIsAgent bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, aiControlTimeout)
	defer cancel()
	res, err := client.GetAIControl(ctx)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden && strings.Contains(apiErr.Message, aiDisabledMessage) {
		return aiModeOff, nil, true, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return res.Mode, res.AllowedEnvKinds, res.CallerIsAgent, nil
}

// aiControlAllows reports whether the mode lets the tool be listed or called. This only trims what the model sees: the server enforces the mode on every request.
func aiControlAllows(mode, tool string) bool {
	if tool == aiControlStatusTool {
		return true
	}
	switch mode {
	case aiModeOff:
		return false
	case aiModeObserve:
		m, ok := toolTable[tool]
		return ok && m.Class == ClassRead
	default:
		return true
	}
}

// aiControlMiddleware hides tools above the current AI control mode, but only for an agent token, the only kind the server governs. When the mode cannot be read it leaves the list alone, since the server still gates every call.
func aiControlMiddleware(client *apiclient.Client) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/list" {
				return next(ctx, method, req)
			}
			mode, _, isAgent, err := fetchAIControl(ctx, client)
			if err != nil || !isAgent {
				return next(ctx, method, req)
			}
			res, err := next(ctx, method, req)
			list, ok := res.(*mcp.ListToolsResult)
			if err != nil || !ok {
				return res, err
			}
			filtered := *list
			filtered.Tools = make([]*mcp.Tool, 0, len(list.Tools))
			for _, t := range list.Tools {
				if aiControlAllows(mode, t.Name) {
					filtered.Tools = append(filtered.Tools, t)
				}
			}
			return &filtered, nil
		}
	}
}
