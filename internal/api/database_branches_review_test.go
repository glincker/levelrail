package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestBranchAPI_RemoteNodeParentIsRefused(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	seedNode(t, db, "node-r", "remote-1")
	if err := db.UpdateDatabaseNode(context.Background(), "main", "node-r"); err != nil {
		t.Fatal(err)
	}
	rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"main-try","mode":"schema"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "branch_remote_node_unsupported") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDatabaseBranch(context.Background(), "main-try"); err == nil {
		t.Fatal("a row was written for a refused branch")
	}
}

func TestBranchAPI_PromotedBranchBecomesRootOfItsTree(t *testing.T) {
	rt, db, cookie, _ := newLiveBranchRouter(t)
	for _, n := range []string{"b1"} {
		if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"`+n+`","mode":"schema"}`); rec.Code != http.StatusAccepted {
			t.Fatalf("create %s = %d", n, rec.Code)
		}
	}
	readyBranch(t, db, "b1")
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches", `{"name":"c1","mode":"schema","parent":"b1"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("child create = %d %s", rec.Code, rec.Body.String())
	}
	readyBranch(t, db, "c1")
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/main/branches/b1/promote", `{"confirm":"b1"}`); rec.Code != http.StatusOK {
		t.Fatalf("promote = %d %s", rec.Code, rec.Body.String())
	}
	child, _ := db.GetDatabaseBranch(context.Background(), "c1")
	if child.RootName != "b1" {
		t.Fatalf("child root = %q, want b1", child.RootName)
	}
	if rec := branchDo(t, rt, cookie, http.MethodGet, "/api/v1/databases/b1/branches/c1", ""); rec.Code != http.StatusOK {
		t.Fatalf("child show under the new root = %d %s", rec.Code, rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/main", ""); rec.Code == http.StatusConflict {
		t.Fatalf("old root delete still blocked: %s", rec.Body.String())
	}
	if rec := branchDo(t, rt, cookie, http.MethodPost, "/api/v1/databases/b1/branches", `{"name":"b1-next","mode":"schema"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("branch from the promoted database = %d %s", rec.Code, rec.Body.String())
	}
}
