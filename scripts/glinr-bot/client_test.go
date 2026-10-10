package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &client{base: srv.URL, token: "t", http: srv.Client()}
}

func TestUpsertCommentUpdatesTheMarkedComment(t *testing.T) {
	var patched string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/o/r/issues/7/comments"):
			_, _ = w.Write([]byte(`[{"id":1,"body":"unrelated"},{"id":2,"body":"` + commentMarker + ` old"}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/o/r/issues/comments/2":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			patched = in["body"]
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	if err := upsertComment(context.Background(), c, "o/r", "7", "new body"); err != nil {
		t.Fatal(err)
	}
	if patched != "new body" {
		t.Errorf("patched body = %q", patched)
	}
}

func TestUpsertCommentCreatesWhenNoneMarked(t *testing.T) {
	posted := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		posted = r.Method == http.MethodPost
		_, _ = w.Write([]byte(`{}`))
	})
	if err := upsertComment(context.Background(), c, "o/r", "7", "b"); err != nil || !posted {
		t.Fatalf("err=%v posted=%v", err, posted)
	}
}

func TestActEnforceApprovesAndArmsAutoMerge(t *testing.T) {
	var reviewBody, graphql string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		b := string(raw)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/reviews"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			reviewBody = b
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/graphql":
			graphql = b
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	v := Verdict{Rule: "docs-only", Actions: []string{ActionApprove, ActionAutomerge}}
	done := act(context.Background(), c, "o/r", "7", ghPull{NodeID: "PR_x"}, v, PR{Author: "someone"})
	if len(done) != 2 || !strings.Contains(reviewBody, `"APPROVE"`) || !strings.Contains(graphql, "enablePullRequestAutoMerge") || !strings.Contains(graphql, "PR_x") {
		t.Errorf("done=%v review=%s graphql=%s", done, reviewBody, graphql)
	}
}

func TestActSkipsApproveOnOwnPR(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) { t.Error("no request expected") })
	done := act(context.Background(), c, "o/r", "7", ghPull{}, Verdict{Actions: []string{ActionApprove}}, PR{SelfAuthored: true})
	if len(done) != 1 || !strings.Contains(done[0], "skipped") {
		t.Errorf("done = %v", done)
	}
}
