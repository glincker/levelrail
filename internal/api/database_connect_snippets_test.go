package api

import (
	"strings"
	"testing"
)

func TestConnectSnippetsMaskAndReveal(t *testing.T) {
	const secret = "s3cr&t'pw" //nolint:gosec // test fixture, not a credential
	engines := []string{"postgres", "mysql", "mariadb", "redis", "valkey", "keydb", "mongodb"}
	for _, eng := range engines {
		tgt := ConnectSnippetTarget{Engine: eng, Host: "db.internal", Port: 5432, User: "app", Database: "main", TLS: eng == "redis"}
		masked := BuildConnectSnippets(tgt, secret, false)
		if len(masked) == 0 {
			t.Fatalf("%s: no snippets", eng)
		}
		for _, s := range masked {
			if strings.Contains(s.Code, "s3cr") {
				t.Errorf("%s/%s leaks password for a masked viewer: %s", eng, s.ID, s.Code)
			}
			if !strings.Contains(s.Code, connectSnippetPlaceholder) {
				t.Errorf("%s/%s lacks placeholder: %s", eng, s.ID, s.Code)
			}
		}
		revealed := BuildConnectSnippets(tgt, secret, true)
		found := false
		for _, s := range revealed {
			if strings.Contains(s.Code, "s3cr") {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: revealed snippets never carry the password", eng)
		}
	}
	if got := BuildConnectSnippets(ConnectSnippetTarget{Engine: "unknown"}, "x", true); got != nil {
		t.Errorf("unknown engine should yield nil, got %v", got)
	}
}

func TestConnectSnippetsPostgresURL(t *testing.T) {
	tgt := ConnectSnippetTarget{Engine: "postgres", Host: "h", Port: 5432, User: "u", Database: "d", TLS: true}
	got := BuildConnectSnippets(tgt, "", false)[0].Code
	want := "postgresql://u:<password>@h:5432/d?sslmode=require"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestConnectSnippetsDocumentDBUsesMongoScheme(t *testing.T) {
	tgt := ConnectSnippetTarget{Engine: "documentdb", Host: "db.internal", Port: 10260, User: "app", Database: "main"}
	for _, s := range BuildConnectSnippets(tgt, "pw", true) {
		if s.ID != "url" {
			continue
		}
		if !strings.HasPrefix(s.Code, "mongodb://") || !strings.Contains(s.Code, "tls=true") || !strings.Contains(s.Code, "tlsAllowInvalidCertificates=true") {
			t.Errorf("url = %s", s.Code)
		}
		return
	}
	t.Fatal("no url snippet")
}

func TestConnectSnippetsDynamoDBHasEndpointAndDummyKeys(t *testing.T) {
	tgt := ConnectSnippetTarget{Engine: "dynamodb", Host: "db-kv", Port: 8000}
	got := map[string]string{}
	for _, s := range BuildConnectSnippets(tgt, "", true) {
		got[s.ID] = s.Code
	}
	if got["url"] != "http://db-kv:8000" {
		t.Errorf("url = %q", got["url"])
	}
	for _, id := range []string{"spring", "spring-cloud", "node", "python", "go"} {
		if !strings.Contains(got[id], "http://db-kv:8000") {
			t.Errorf("%s snippet missing endpoint: %q", id, got[id])
		}
	}
}
