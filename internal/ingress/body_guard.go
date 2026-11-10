package ingress

import (
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(bodyGuardModule{})
}

// BodyGuardHandler is the wire shape of the levelrail_body_guard handler.
type BodyGuardHandler struct {
	Handler string `json:"handler"`
	MaxSize int64  `json:"max_size"`
}

// bodyGuardModule (http.handlers.levelrail_body_guard) answers 413 from the
// declared Content-Length before any body byte is read or forwarded upstream.
type bodyGuardModule struct {
	MaxSize int64 `json:"max_size"`
}

func (bodyGuardModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_body_guard",
		New: func() caddy.Module { return new(bodyGuardModule) },
	}
}

func (m *bodyGuardModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if m.MaxSize > 0 && r.ContentLength > m.MaxSize {
		return caddyhttp.Error(http.StatusRequestEntityTooLarge, http.ErrContentLength)
	}
	return next.ServeHTTP(w, r)
}
