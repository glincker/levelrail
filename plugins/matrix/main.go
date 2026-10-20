// Command matrix posts alerts to a Matrix room with a bot access token.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "matrix"
	pluginVersion = "0.1.0"
	maxText       = 8000
)

var httpc pluginkit.HTTP

type config struct {
	Homeserver string `json:"homeserver"`
	RoomID     string `json:"room_id"`
	AllowHTTP  bool   `json:"allow_http"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion, Send: send,
		ConfigSchema: `{"type":"object","additionalProperties":false,"required":["homeserver","room_id"],"properties":{"homeserver":{"type":"string","description":"Homeserver base URL, e.g. https://matrix.example.com"},"room_id":{"type":"string","description":"Internal room id such as !abc:example.com"},"allow_http":{"type":"boolean","description":"Permit plain http for a lab server"}}}`,
	}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg config
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	base, err := pluginkit.BaseURL(cfg.Homeserver, cfg.AllowHTTP)
	if err != nil {
		return pluginkit.Invalid(err)
	}
	if !strings.HasPrefix(cfg.RoomID, "!") {
		return pluginkit.Invalid(errors.New("invalid config: room_id must be the internal room id, which starts with an exclamation mark"))
	}
	if strings.TrimSpace(c.Secret) == "" {
		return pluginkit.Invalid(errors.New("the channel target must be the bot access token"))
	}
	text := pluginkit.Truncate(pluginkit.Text(c.Event), maxText)
	body, _ := json.Marshal(map[string]string{"msgtype": "m.text", "body": text})
	// A stable transaction id makes a retried send idempotent on the server.
	sum := sha256.Sum256([]byte(c.Event.GetId() + "\x00" + text))
	endpoint := base.String() + "/_matrix/client/v3/rooms/" + url.PathEscape(cfg.RoomID) + "/send/m.room.message/" + hex.EncodeToString(sum[:16])
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Secret)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, nil, c.Secret)
}

func main() { pluginkit.Main(newPlugin()) }
