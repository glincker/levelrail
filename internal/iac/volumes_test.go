package iac

import (
	"strings"
	"testing"
)

const volumeApp = `version: 1
kind: App
metadata: {name: db-app}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/db:1"}
    port: 8080
    volumes:
      - {name: data, path: /data}
`

func TestVolumesApplyByNameKeepAndPrune(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git"}
	applyAll(t, f, volumeApp, opts)
	if !hasWrite(f, "PUT /api/v1/apps/db-app/volumes") {
		t.Fatalf("volumes not attached: %v", f.writes)
	}
	if p := planFor(t, f, volumeApp, opts); p.Summary.Create+p.Summary.Update+p.Summary.Delete != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}

	more := strings.Replace(volumeApp, "/data}\n", "/data}\n      - {name: cache, path: /cache}\n", 1)
	f.writes = nil
	applyAll(t, f, more, opts)
	if !hasWrite(f, "POST /api/v1/apps/db-app/restart") || len(stringAny(f.apps["db-app"]["volumes"])) != 2 {
		t.Fatalf("add volume: %v %v", f.writes, f.apps["db-app"]["volumes"])
	}

	// Listing fewer volumes without prune detaches nothing.
	f.writes = nil
	applyAll(t, f, volumeApp, opts)
	if len(stringAny(f.apps["db-app"]["volumes"])) != 2 {
		t.Fatalf("unlisted volume dropped without prune: %v", f.apps["db-app"]["volumes"])
	}
	applyAll(t, f, volumeApp, Options{Source: "git", Prune: true})
	if len(stringAny(f.apps["db-app"]["volumes"])) != 1 {
		t.Fatalf("prune did not detach: %v", f.apps["db-app"]["volumes"])
	}
	for _, w := range f.writes {
		if strings.HasPrefix(w, "DELETE") && strings.Contains(w, "volume") {
			t.Fatalf("a volume was deleted: %s", w)
		}
	}
}

func TestHostPathVolumeRejected(t *testing.T) {
	src := strings.Replace(volumeApp, "{name: data, path: /data}", "{hostPath: /srv/x, path: /data}", 1)
	docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(src)}})
	if len(issues) == 0 {
		_, issues = Build(docs, Options{})
	}
	if len(issues) == 0 {
		t.Fatal("host path volume accepted")
	}
}
