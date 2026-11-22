package alerting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Matrix and Zulip are built-in kinds. Each stores its whole target in
// notify_url, so the existing sealing, masking and SSRF-guarded client apply:
//
//	matrix: https://matrix.example.com/?room=!abc:example.com&token=ACCESS_TOKEN
//	zulip:  https://bot%40example.com:API_KEY@zulip.example.com/?stream=ops&topic=alerts
const (
	chatMaxText      = 8000
	zulipDefaultTopc = "alerts"
)

type chatTarget struct {
	base     string
	room     string
	token    string
	botEmail string
	apiKey   string
	stream   string
	topic    string
}

func parseMatrixTarget(raw string) (chatTarget, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return chatTarget{}, errors.New("matrix notify_url must be the homeserver URL with room and token query parameters")
	}
	q := u.Query()
	room, token := q.Get("room"), q.Get("token")
	if !strings.HasPrefix(room, "!") {
		return chatTarget{}, errors.New("matrix room must be the internal room id, which starts with an exclamation mark")
	}
	if strings.TrimSpace(token) == "" {
		return chatTarget{}, errors.New("matrix notify_url needs a token query parameter holding the bot access token")
	}
	return chatTarget{base: u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), room: room, token: token}, nil
}

func parseZulipTarget(raw string) (chatTarget, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return chatTarget{}, errors.New("zulip notify_url must be the server URL with the bot credentials and a stream")
	}
	pass, _ := u.User.Password()
	if u.User.Username() == "" || pass == "" {
		return chatTarget{}, errors.New("zulip notify_url needs the bot email and API key as user:password")
	}
	q := u.Query()
	if strings.TrimSpace(q.Get("stream")) == "" {
		return chatTarget{}, errors.New("zulip notify_url needs a stream query parameter")
	}
	topic := q.Get("topic")
	if topic == "" {
		topic = zulipDefaultTopc
	}
	return chatTarget{
		base: u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), botEmail: u.User.Username(), apiKey: pass,
		stream: q.Get("stream"), topic: topic,
	}, nil
}

// sendChatText delivers text to a Matrix room or Zulip stream, retrying only
// transient failures like postJSONWithAuth does.
func sendChatText(ctx context.Context, client *http.Client, kind NotifyKind, target, text string) error {
	if target == "" {
		return fmt.Errorf("alerting: notify: no notify_url configured")
	}
	if IsMaskedNotifyTarget(target) {
		return ErrChannelSecretNotRevealed
	}
	if len(text) > chatMaxText {
		text = text[:chatMaxText]
	}
	build, err := chatRequestBuilder(kind, target, text)
	if err != nil {
		return fmt.Errorf("alerting: notify: %w", err)
	}
	delay := notifyRetryBaseDelay
	var lastErr error
	for attempt := 1; attempt <= notifyMaxAttempts; attempt++ {
		lastErr = doChatRequest(ctx, client, build)
		if lastErr == nil || attempt == notifyMaxAttempts || !isRetryableNotifyError(lastErr) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return lastErr
		case <-time.After(delay):
		}
		delay *= 2
	}
	return lastErr
}

func chatRequestBuilder(kind NotifyKind, target, text string) (func(context.Context) (*http.Request, error), error) {
	if kind == NotifyMatrix {
		t, err := parseMatrixTarget(target)
		if err != nil {
			return nil, err
		}
		body, _ := json.Marshal(map[string]string{"msgtype": "m.text", "body": text})
		// A stable transaction id makes a retried send idempotent on the server.
		sum := sha256.Sum256([]byte(text + time.Now().UTC().Format(time.RFC3339Nano)))
		endpoint := t.base + "/_matrix/client/v3/rooms/" + url.PathEscape(t.room) + "/send/m.room.message/" + hex.EncodeToString(sum[:16])
		return func(ctx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", bearerPrefix+t.token)
			req.Header.Set(headerContentType, contentTypeJSON)
			return req, nil
		}, nil
	}
	t, err := parseZulipTarget(target)
	if err != nil {
		return nil, err
	}
	form := url.Values{"type": {"stream"}, "to": {t.stream}, "topic": {truncateChat(t.topic, 60)}, "content": {text}}.Encode()
	return func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.base+"/api/v1/messages", strings.NewReader(form))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(t.botEmail, t.apiKey)
		req.Header.Set(headerContentType, "application/x-www-form-urlencoded")
		return req, nil
	}, nil
}

func doChatRequest(ctx context.Context, client *http.Client, build func(context.Context) (*http.Request, error)) error {
	req, err := build(ctx)
	if err != nil {
		return fmt.Errorf("alerting: notify: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("alerting: notify: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &notifyHTTPError{statusCode: resp.StatusCode}
	}
	return nil
}

func truncateChat(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func notifyMatrix(ctx context.Context, client *http.Client, target string, ev Event) error {
	return sendChatText(ctx, client, NotifyMatrix, target, summaryText(ev))
}

func notifyZulip(ctx context.Context, client *http.Client, target string, ev Event) error {
	return sendChatText(ctx, client, NotifyZulip, target, summaryText(ev))
}
