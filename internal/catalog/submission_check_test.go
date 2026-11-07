package catalog

import (
	"io/fs"
	"testing"
)

func TestCheckSubmissionAcceptsShippedManifest(t *testing.T) {
	sub, err := fs.Sub(manifestFS, manifestDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"mongodb", "cassandra", "kafka", "emqx", "gokapi", "otel-lgtm", "solr", "nginx"} {
		tpl, err := fs.ReadFile(sub, id+"/template.yaml")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		cmp, err := fs.ReadFile(sub, id+"/compose.yaml")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if _, err := CheckSubmission(tpl, cmp); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

func TestCheckSubmissionRejectsLatestTag(t *testing.T) {
	sub, err := fs.Sub(manifestFS, manifestDir)
	if err != nil {
		t.Fatal(err)
	}
	tpl, _ := fs.ReadFile(sub, "nginx/template.yaml")
	bad := []byte("services:\n  nginx:\n    image: nginx:latest\n    ports: [\"80\"]\n")
	if _, err := CheckSubmission(tpl, bad); err == nil {
		t.Fatal("expected an error for an unpinned image")
	}
}

func TestSearchTemplates(t *testing.T) {
	all := []Template{{ID: "mongodb", Name: "MongoDB", Category: "Database Tools"}, {ID: "nginx", Name: "Nginx", Category: "Infrastructure"}}
	if got := SearchTemplates(all, "database mongo"); len(got) != 1 || got[0].ID != "mongodb" {
		t.Fatalf("got %v", got)
	}
	if got := SearchTemplates(all, "  "); len(got) != 0 {
		t.Fatalf("empty query matched %v", got)
	}
}
