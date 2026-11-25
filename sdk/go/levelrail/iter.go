package levelrail

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"strings"
)

// Paginate walks a cursor-paged list route. fetch returns one page and the
// next cursor; iteration stops at an empty cursor, an error or an early break.
func Paginate[T any](ctx context.Context, fetch func(ctx context.Context, cursor string) ([]T, string, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		cursor := ""
		for {
			if err := ctx.Err(); err != nil {
				var zero T
				yield(zero, err)
				return
			}
			items, next, err := fetch(ctx, cursor)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, it := range items {
				if !yield(it, nil) {
					return
				}
			}
			// A repeated cursor would loop forever on a misbehaving server.
			if next == "" || next == cursor {
				return
			}
			cursor = next
		}
	}
}

// AllApps iterates every app, fetching pages of o.Limit on demand.
func (c *Client) AllApps(ctx context.Context, o ListOptions) iter.Seq2[Object, error] {
	return Paginate(ctx, func(ctx context.Context, cursor string) ([]Object, string, error) {
		o.Cursor = cursor
		return c.ListApps(ctx, o)
	})
}

// SSEEvent is one server-sent event frame.
type SSEEvent struct {
	Event string
	ID    string
	Data  string
}

// ErrFrameTooLarge means one SSE frame exceeded the byte cap.
var ErrFrameTooLarge = errors.New("levelrail: SSE frame too large")

// ReadSSE parses a text/event-stream body with the default frame cap.
func ReadSSE(r io.Reader) iter.Seq2[SSEEvent, error] {
	return ReadSSEMax(r, DefaultMaxFrameBytes)
}

// ReadSSEMax parses a text/event-stream body. Comment lines are skipped and a
// frame over maxFrame bytes (lines without a blank separator) is an error.
func ReadSSEMax(r io.Reader, maxFrame int) iter.Seq2[SSEEvent, error] {
	return func(yield func(SSEEvent, error) bool) {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		var ev SSEEvent
		var data []string
		size := 0
		flush := func() bool {
			if ev.Event == "" && ev.ID == "" && len(data) == 0 {
				return true
			}
			ev.Data = strings.Join(data, "\n")
			out := ev
			ev, data, size = SSEEvent{}, nil, 0
			return yield(out, nil)
		}
		for sc.Scan() {
			line := sc.Text()
			if size += len(line) + 1; maxFrame > 0 && size > maxFrame {
				yield(SSEEvent{}, ErrFrameTooLarge)
				return
			}
			if line == "" {
				if !flush() {
					return
				}
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				ev.Event = value
			case "id":
				ev.ID = value
			case "data":
				data = append(data, value)
			}
		}
		if err := sc.Err(); err != nil {
			yield(SSEEvent{}, err)
			return
		}
		flush()
	}
}

// ChangeEvent is a nudge naming the topics that changed, or a heartbeat.
type ChangeEvent struct {
	Topics      []string
	Heartbeat   bool
	HeartbeatMS int64
}

// Changes streams GET /api/v1/events/changes until ctx ends or the server
// closes the stream. Callers reconnect and refetch on error or end of stream.
func (c *Client) Changes(ctx context.Context) iter.Seq2[ChangeEvent, error] {
	return func(yield func(ChangeEvent, error) bool) {
		resp, err := c.request(ctx, "handleChangesStream", nil, nil, nil, "text/event-stream")
		if err != nil {
			yield(ChangeEvent{}, err)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		for raw, err := range ReadSSEMax(resp.Body, c.frame) {
			if err != nil {
				yield(ChangeEvent{}, err)
				return
			}
			var ce ChangeEvent
			if raw.Event == "ping" {
				var p struct {
					HeartbeatMS int64 `json:"heartbeat_ms"`
				}
				_ = json.Unmarshal([]byte(raw.Data), &p)
				ce = ChangeEvent{Heartbeat: true, HeartbeatMS: p.HeartbeatMS}
			} else {
				var m struct {
					Topics []string `json:"topics"`
				}
				if json.Unmarshal([]byte(raw.Data), &m) != nil {
					continue
				}
				ce = ChangeEvent{Topics: m.Topics}
			}
			if !yield(ce, nil) {
				return
			}
		}
	}
}
