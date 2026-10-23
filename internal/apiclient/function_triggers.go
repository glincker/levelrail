package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// FunctionWebhook is a function's signed webhook trigger. Secret is set only when one was just generated.
type FunctionWebhook struct {
	URL                 string     `json:"url"`
	Path                string     `json:"path"`
	ReplayWindowSeconds int        `json:"replay_window_seconds"`
	CreatedAt           time.Time  `json:"created_at"`
	RotatedAt           *time.Time `json:"rotated_at,omitempty"`
	Secret              string     `json:"secret,omitempty"`
}

// FunctionSchedule is an http-mode scheduled job that calls the function.
type FunctionSchedule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Timezone string `json:"timezone,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// FunctionTriggers is the GET /api/v1/functions/{name}/triggers response.
type FunctionTriggers struct {
	Function  string             `json:"function"`
	Webhook   *FunctionWebhook   `json:"webhook"`
	Schedules []FunctionSchedule `json:"schedules"`
	Queue     map[string]int     `json:"queue"`
}

// QueuedFunctionCall is one queued, finished or dead-lettered call.
type QueuedFunctionCall struct {
	ID             string    `json:"id"`
	Function       string    `json:"function"`
	Source         string    `json:"source"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	BodyBytes      int       `json:"body_bytes"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	MaxAttempts    int       `json:"max_attempts"`
	NextAttemptAt  time.Time `json:"next_attempt_at"`
	LastStatusCode int       `json:"last_status_code,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// GetFunctionTriggers lists the triggers of one function.
func (c *Client) GetFunctionTriggers(ctx context.Context, name string) (FunctionTriggers, error) {
	var out FunctionTriggers
	err := c.do(ctx, http.MethodGet, "/api/v1/functions/"+url.PathEscape(name)+"/triggers", nil, &out)
	return out, err
}

// PutFunctionWebhook enables, retargets or rotates the signed webhook of a function.
func (c *Client) PutFunctionWebhook(ctx context.Context, name, path string, rotate bool) (FunctionWebhook, error) {
	var out FunctionWebhook
	body := map[string]any{"path": path, "rotate": rotate}
	err := c.do(ctx, http.MethodPut, "/api/v1/functions/"+url.PathEscape(name)+"/triggers/webhook", body, &out)
	return out, err
}

// DeleteFunctionWebhook turns the webhook trigger off.
func (c *Client) DeleteFunctionWebhook(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/functions/"+url.PathEscape(name)+"/triggers/webhook", nil, nil)
}

// EnqueueFunctionCall queues an async call and returns at once.
func (c *Client) EnqueueFunctionCall(ctx context.Context, name, method, path, body string) (QueuedFunctionCall, error) {
	var out QueuedFunctionCall
	req := map[string]string{"method": method, "path": path, "body": body}
	err := c.do(ctx, http.MethodPost, "/api/v1/functions/"+url.PathEscape(name)+"/async", req, &out)
	return out, err
}

// ListQueuedFunctionCalls lists calls; empty function or status match any.
func (c *Client) ListQueuedFunctionCalls(ctx context.Context, function, status string, limit int) ([]QueuedFunctionCall, error) {
	q := url.Values{}
	if function != "" {
		q.Set("function", function)
	}
	if status != "" {
		q.Set("status", status)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out struct {
		Invocations []QueuedFunctionCall `json:"invocations"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/function-invocations?"+q.Encode(), nil, &out)
	return out.Invocations, err
}

// RetryFunctionInvocation puts a dead-lettered call back in the queue.
func (c *Client) RetryFunctionInvocation(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/function-invocations/"+url.PathEscape(id)+"/retry", nil, nil)
}

// DeleteFunctionInvocation discards a call.
func (c *Client) DeleteFunctionInvocation(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/function-invocations/"+url.PathEscape(id), nil, nil)
}
