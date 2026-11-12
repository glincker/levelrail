package iac

import (
	"encoding/json"
	"strings"
	"testing"
)

const secretConnFixture = `version: 1
kind: App
metadata: {name: web}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/web:1"}
    port: 3000
    env:
      A: secret://prod/apps/web#a
      B: {secret: {conn: prod, path: apps/web, key: b}}
      C: {secretRef: {connection: prod, path: apps/web, key: c}}
`

func TestSecretConnRefsPlanShowRefText(t *testing.T) {
	plan := planFor(t, newFake(), secretConnFixture, Options{})
	raw, _ := json.Marshal(plan)
	for _, want := range []string{"secret://prod/apps/web#a", "secret://prod/apps/web#b", "secret://prod/apps/web#c"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("plan lacks %s: %s", want, raw)
		}
	}
}

func TestSecretConnRefInvalidIsIssue(t *testing.T) {
	base := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      K: "
	for _, val := range []string{"secret://Prod/a#k", "{secret: {conn: prod, path: ../x, key: k}}", "{secretRef: {path: a, key: k}}"} {
		docs, issues := ParseDocuments([]Source{{Name: "t.yaml", Data: []byte(base + val + "\n")}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		if len(issues) == 0 {
			t.Fatalf("%s must be an issue", val)
		}
	}
}
