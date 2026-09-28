package provision

import "golang.org/x/oauth2"

// fakeTokenSource is azure_test.go and gcp_test.go's shared stand-in for
// a real OAuth2 token source, returning a fixed token or a fixed error
// without any network call.
type fakeTokenSource struct {
	token *oauth2.Token
	err   error
}

func (f fakeTokenSource) Token() (*oauth2.Token, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.token, nil
}
