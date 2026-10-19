package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// IAMAbilityUse mirrors internal/api's abilityUse.
type IAMAbilityUse struct {
	Ability    string `json:"ability"`
	Requests   int    `json:"requests"`
	LastUsedAt string `json:"last_used_at"`
}

// IAMTokenSuggestion mirrors internal/api's tokenSuggestion.
type IAMTokenSuggestion struct {
	TokenID            string          `json:"token_id"`
	Name               string          `json:"name"`
	AgentName          string          `json:"agent_name,omitempty"`
	Granted            []string        `json:"granted"`
	Used               []IAMAbilityUse `json:"used"`
	Unused             []string        `json:"unused"`
	SuggestedAbilities []string        `json:"suggested_abilities"`
	Status             string          `json:"status"`
	Risk               string          `json:"risk"`
	Reason             string          `json:"reason"`
}

// IAMSuggestions mirrors internal/api's suggestionsResponse.
type IAMSuggestions struct {
	WindowDays int                  `json:"window_days"`
	Tokens     []IAMTokenSuggestion `json:"tokens"`
	Note       string               `json:"note"`
}

// IAMSuggestions calls GET /api/v1/iam/suggestions; days 0 uses the server default.
func (c *Client) IAMSuggestions(ctx context.Context, days int) (IAMSuggestions, error) {
	path := "/api/v1/iam/suggestions"
	if days > 0 {
		path += "?" + url.Values{"days": {strconv.Itoa(days)}}.Encode()
	}
	var out IAMSuggestions
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
