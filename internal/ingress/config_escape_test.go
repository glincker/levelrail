package ingress

import "testing"

func TestPlaceholderEscaping(t *testing.T) {
	tests := []struct{ name, got, want string }{
		{"bare origin keeps uri passthrough", redirectLocation("https://a.example"), "https://a.example{http.request.uri}"},
		{"env placeholder in origin neutralised", redirectLocation("https://a.example/{env.APP_MASTER_KEY}"), `https://a.example/\{env.APP_MASTER_KEY\}`},
		{"file placeholder in query neutralised", redirectLocation("https://a.example/?k={file./etc/passwd}"), `https://a.example/?k=\{file./etc/passwd\}`},
		{"error page body neutralised", NewErrorPageResponse(404, "<p>{env.X}</p>").Body, `<p>\{env.X\}</p>`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q want %q", tc.got, tc.want)
			}
		})
	}
}
