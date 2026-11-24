package imageupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPTagListerRefusesRedirectAndPrivateRealm(t *testing.T) {
	redir := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1:2019/v2/x/tags/list", http.StatusFound)
	}))
	defer redir.Close()
	host := strings.TrimPrefix(redir.URL, "https://")
	_, err := HTTPTagLister{Client: redir.Client()}.ListTags(context.Background(), host+"/team/app:1", nil)
	if err == nil || err.Error() != ErrRegistryRequest.Error() {
		t.Fatalf("redirect error = %v, want generic", err)
	}

	realm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Www-Authenticate", `Bearer realm="https://10.0.0.5:5000/token",service="r"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer realm.Close()
	host = strings.TrimPrefix(realm.URL, "https://")
	_, err = HTTPTagLister{Client: realm.Client()}.ListTags(context.Background(), host+"/team/app:1", nil)
	if err == nil || err.Error() != ErrRegistryRequest.Error() {
		t.Fatalf("realm error = %v, want generic", err)
	}
}
