package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// The types below mirror internal/deploytimeline's wire shape. They are
// copied rather than imported so the CLI does not link the store.

// DeployEvent is one entry on a deploy's event timeline.
type DeployEvent struct {
	ID         string              `json:"id"`
	At         time.Time           `json:"at"`
	Kind       string              `json:"kind"`
	Phase      string              `json:"phase"`
	Status     string              `json:"status"`
	Code       string              `json:"code"`
	Title      string              `json:"title"`
	Detail     string              `json:"detail,omitempty"`
	Reason     string              `json:"reason,omitempty"`
	Service    string              `json:"service,omitempty"`
	Stream     string              `json:"stream,omitempty"`
	DurationMS int64               `json:"duration_ms,omitempty"`
	Trigger    *DeployEventTrigger `json:"trigger,omitempty"`
	Impact     *DeployEventImpact  `json:"impact,omitempty"`
}

// DeployEventTrigger says who or what started the deploy.
type DeployEventTrigger struct {
	Source        string `json:"source"`
	Actor         string `json:"actor,omitempty"`
	ActorKind     string `json:"actor_kind,omitempty"`
	Branch        string `json:"branch,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	Author        string `json:"author,omitempty"`
	RerunOf       string `json:"rerun_of,omitempty"`
}

// DeployEventWindow is request health over one slice of time.
type DeployEventWindow struct {
	HasTraffic   bool    `json:"has_traffic"`
	Requests     float64 `json:"requests"`
	RatePerSec   float64 `json:"rate_per_sec"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P95Ms        float64 `json:"p95_ms"`
}

// DeployEventImpact compares request health before and after cutover.
type DeployEventImpact struct {
	CutoverAt time.Time         `json:"cutover_at"`
	WindowMS  int64             `json:"window_ms"`
	Complete  bool              `json:"complete"`
	Before    DeployEventWindow `json:"before"`
	After     DeployEventWindow `json:"after"`
}

// DeployEventPhase is the time one phase took.
type DeployEventPhase struct {
	Phase      string    `json:"phase"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMS int64     `json:"duration_ms"`
}

// DeployEventTimeline is the whole story of one deploy.
type DeployEventTimeline struct {
	DeployID    string             `json:"deploy_id"`
	App         string             `json:"app"`
	Verdict     string             `json:"verdict"`
	Live        bool               `json:"live"`
	Frozen      bool               `json:"frozen"`
	StartedAt   time.Time          `json:"started_at"`
	FinishedAt  *time.Time         `json:"finished_at,omitempty"`
	Trigger     DeployEventTrigger `json:"trigger"`
	Events      []DeployEvent      `json:"events"`
	Phases      []DeployEventPhase `json:"phases"`
	Impact      *DeployEventImpact `json:"impact,omitempty"`
	LogsDropped int                `json:"logs_dropped,omitempty"`
}

// DeployEventMessage is one message on the timeline stream: snapshot, event,
// state or end.
type DeployEventMessage struct {
	Type     string               `json:"type"`
	Timeline *DeployEventTimeline `json:"timeline,omitempty"`
	Event    *DeployEvent         `json:"event,omitempty"`
	Verdict  string               `json:"verdict,omitempty"`
	Live     bool                 `json:"live"`
	Frozen   bool                 `json:"frozen,omitempty"`
	Phases   []DeployEventPhase   `json:"phases,omitempty"`
}

// DeployEventPhaseDelta lines up one phase of two deploys.
type DeployEventPhaseDelta struct {
	Phase   string `json:"phase"`
	AMS     *int64 `json:"a_ms,omitempty"`
	BMS     *int64 `json:"b_ms,omitempty"`
	DeltaMS *int64 `json:"delta_ms,omitempty"`
}

// DeployEventComparison puts two timelines next to each other.
type DeployEventComparison struct {
	A      DeployEventTimeline     `json:"a"`
	B      DeployEventTimeline     `json:"b"`
	Phases []DeployEventPhaseDelta `json:"phases"`
	OnlyA  []string                `json:"only_a"`
	OnlyB  []string                `json:"only_b"`
}

// DeployEventTimeline calls GET /api/v1/deployments/{id}/timeline.
func (c *Client) DeployEventTimeline(ctx context.Context, id string) (DeployEventTimeline, error) {
	var out DeployEventTimeline
	err := c.do(ctx, http.MethodGet, "/api/v1/deployments/"+PathEscape(id)+"/timeline", nil, &out)
	return out, err
}

// StreamDeployEventTimeline calls GET /api/v1/deployments/{id}/timeline/stream
// and hands each message to onMsg until onMsg returns an error, the server
// sends its end message and closes, or ctx ends.
func (c *Client) StreamDeployEventTimeline(ctx context.Context, id string, onMsg func(DeployEventMessage) error) error {
	return streamSSE(ctx, c, http.MethodGet, "/api/v1/deployments/"+PathEscape(id)+"/timeline/stream", nil, onMsg)
}

// CompareDeployEventTimelines calls
// GET /api/v1/deployments/{id}/timeline/compare?with={other}.
func (c *Client) CompareDeployEventTimelines(ctx context.Context, id, other string) (DeployEventComparison, error) {
	var out DeployEventComparison
	path := "/api/v1/deployments/" + PathEscape(id) + "/timeline/compare?" + url.Values{"with": {other}}.Encode()
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
