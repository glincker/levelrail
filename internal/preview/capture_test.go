package preview

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"testing"
)

func encodePNG(w io.Writer, img image.Image) error { return png.Encode(w, img) }

func TestShotResultClassify(t *testing.T) {
	tests := []struct {
		name      string
		res       ShotResult
		path      string
		wantCause string
	}{
		{"ok", ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/"}, "/", ""},
		{"ok after i18n redirect", ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/en"}, "/", ""},
		{"unreachable", ShotResult{}, "/", ReasonUnreachable},
		{"unauthorized", ShotResult{HTTPStatus: 401}, "/", ReasonAuthWall},
		{"forbidden", ShotResult{HTTPStatus: 403}, "/", ReasonAuthWall},
		{"not found", ShotResult{HTTPStatus: 404}, "/", ReasonHTTPStatus},
		{"server error", ShotResult{HTTPStatus: 500}, "/", ReasonHTTPStatus},
		{"redirect status left over", ShotResult{HTTPStatus: 302}, "/", ReasonHTTPStatus},
		{"login redirect", ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/login?next=/"}, "/", ReasonAuthWall},
		{"sso redirect", ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/sso/start"}, "/dashboard", ReasonAuthWall},
		{"login page requested on purpose", ShotResult{HTTPStatus: 200, FinalURL: "http://web:3000/login"}, "/login", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := tt.res.Classify(tt.path)
			if got != tt.wantCause {
				t.Errorf("Classify = %q, want %q", got, tt.wantCause)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	good := []string{"/", "/pricing", "/a/b?x=1&y=2", "/docs#top", "/en-US/home"}
	bad := []string{"", "pricing", "//evil.example", "http://evil.example/", "/a b", "/a\nb", "/\\evil", string(bytes.Repeat([]byte("a"), 600))}
	for _, p := range good {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", p)
		}
	}
}
