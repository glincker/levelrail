package main

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestPlanFromFileKeepsRegistryCredential(t *testing.T) {
	fileSpec, err := spec.Parse([]byte(`version: 1
services:
  web:
    port: 80
    build:
      type: image
      image: registry.example.com/team/web:1
      registryCredential: gitlab
`))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planFromFile(createFlags{file: "app.yaml"}, fileSpec, detectedGit{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.CreateBody.RegistryCredential != "gitlab" {
		t.Fatalf("RegistryCredential = %q, want gitlab", plan.CreateBody.RegistryCredential)
	}
}
