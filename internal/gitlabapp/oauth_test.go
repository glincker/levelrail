package gitlabapp

import (
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	tests := []struct {
		name        string
		instanceURL string
		clientID    string
		redirectURI string
		state       string
		want        string
	}{
		{
			name:        "gitlab.com",
			instanceURL: "https://gitlab.com",
			clientID:    "abc",
			redirectURI: "https://example.com/cb",
			state:       "xyz",
			want:        "https://gitlab.com/oauth/authorize?client_id=abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&scope=api&state=xyz",
		},
		{
			name:        "self-hosted with trailing slash",
			instanceURL: "https://gitlab.internal.example.com/",
			clientID:    "abc",
			redirectURI: "https://example.com/cb",
			state:       "xyz",
			want:        "https://gitlab.internal.example.com/oauth/authorize?client_id=abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&scope=api&state=xyz",
		},
		{
			name:        "empty instance URL",
			instanceURL: "",
			clientID:    "abc",
			redirectURI: "https://example.com/cb",
			state:       "xyz",
			want:        "/oauth/authorize?client_id=abc&redirect_uri=https%3A%2F%2Fexample.com%2Fcb&response_type=code&scope=api&state=xyz",
		},
		{
			name:        "different client ID and redirect URI",
			instanceURL: "https://gitlab.com",
			clientID:    "def",
			redirectURI: "https://app.example.com/oauth/callback",
			state:       "123",
			want:        "https://gitlab.com/oauth/authorize?client_id=def&redirect_uri=https%3A%2F%2Fapp.example.com%2Foauth%2Fcallback&response_type=code&scope=api&state=123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AuthorizeURL(tt.instanceURL, tt.clientID, tt.redirectURI, tt.state)
			if got != tt.want {
				t.Errorf("AuthorizeURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
