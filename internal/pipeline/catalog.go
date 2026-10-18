package pipeline

import (
	"fmt"
	"sort"
)

// CatalogTemplate is a ready-to-save starter pipeline file.
type CatalogTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Language    string   `json:"language"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	YAML        string   `json:"yaml,omitempty"`
}

// Catalog returns the built-in starter pipelines sorted by language then name.
func Catalog() []CatalogTemplate {
	out := make([]CatalogTemplate, len(catalog))
	copy(out, catalog)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Language != out[j].Language {
			return out[i].Language < out[j].Language
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// CatalogByID looks up one starter pipeline.
func CatalogByID(id string) (CatalogTemplate, error) {
	for _, t := range catalog {
		if t.ID == id {
			return t, nil
		}
	}
	return CatalogTemplate{}, fmt.Errorf("pipeline: unknown template %q", id)
}

var catalog = []CatalogTemplate{
	{
		ID: "node-ci", Name: "Node test and build", Language: "node",
		Description: "Install, lint, test, then build the app image on every push and pull request.",
		Tags:        []string{"test", "build"},
		YAML: `version: 1
name: node-ci
on:
  push:
    branches: [main]
  pull_request: {}
  manual: {}
stages: [test, build]
jobs:
  test:
    stage: test
    image: node:22
    retries: 1
    steps:
      - run: npm ci
      - run: npm run lint --if-present
      - run: npm test --if-present
  build:
    stage: build
    steps:
      - uses: build
        id: image
`,
	},
	{
		ID: "node-deploy", Name: "Node test, build and deploy", Language: "node",
		Description: "Test, build, ask for approval, then deploy blue-green from main.",
		Tags:        []string{"test", "build", "deploy", "approval"},
		YAML: `version: 1
name: node-release
on:
  push:
    branches: [main]
  manual: {}
stages: [test, build, deploy]
jobs:
  test:
    stage: test
    image: node:22
    steps:
      - run: npm ci
      - run: npm test --if-present
  build:
    stage: build
    steps:
      - uses: build
        id: image
  ship:
    stage: deploy
    steps:
      - uses: approval
        with:
          message: Deploy to production?
          approvers: deploy
      - uses: deploy
        with:
          strategy: blue-green
`,
	},
	{
		ID: "node-matrix", Name: "Node version matrix", Language: "node",
		Description: "Run the test suite against several Node versions in parallel.",
		Tags:        []string{"test", "matrix"},
		YAML: `version: 1
name: node-matrix
on:
  pull_request: {}
  push:
    branches: [main]
jobs:
  test:
    image: node:${{ matrix.node }}
    matrix:
      node: ["20", "22"]
    steps:
      - run: npm ci
      - run: npm test --if-present
`,
	},
	{
		ID: "go-ci", Name: "Go vet and test", Language: "go",
		Description: "Vet and test a Go module, then build the app image.",
		Tags:        []string{"test", "build"},
		YAML: `version: 1
name: go-ci
on:
  push:
    branches: [main]
  pull_request: {}
  manual: {}
stages: [test, build]
jobs:
  test:
    stage: test
    image: golang:1.23
    steps:
      - run: go vet ./...
      - run: go test ./... -short
  build:
    stage: build
    steps:
      - uses: build
        id: image
`,
	},
	{
		ID: "go-release", Name: "Go tagged release", Language: "go",
		Description: "On a version tag, test, build and deploy after approval.",
		Tags:        []string{"release", "deploy", "approval"},
		YAML: `version: 1
name: go-release
on:
  tag:
    patterns: ["v*"]
stages: [test, build, deploy]
jobs:
  test:
    stage: test
    image: golang:1.23
    steps:
      - run: go test ./...
  build:
    stage: build
    steps:
      - uses: build
        id: image
  ship:
    stage: deploy
    steps:
      - uses: approval
        with:
          message: Ship this tag?
          approvers: deploy
      - uses: deploy
`,
	},
	{
		ID: "python-ci", Name: "Python test and build", Language: "python",
		Description: "Install requirements, run pytest, then build the app image.",
		Tags:        []string{"test", "build"},
		YAML: `version: 1
name: python-ci
on:
  push:
    branches: [main]
  pull_request: {}
  manual: {}
stages: [test, build]
jobs:
  test:
    stage: test
    image: python:3.12
    retries: 1
    steps:
      - run: pip install -r requirements.txt
      - run: pip install pytest
      - run: pytest -q
  build:
    stage: build
    steps:
      - uses: build
        id: image
`,
	},
	{
		ID: "static-site", Name: "Static site build and deploy", Language: "static",
		Description: "Build a static site with npm and deploy on every push to main.",
		Tags:        []string{"build", "deploy"},
		YAML: `version: 1
name: static-site
on:
  push:
    branches: [main]
  manual: {}
stages: [build, deploy]
jobs:
  site:
    stage: build
    image: node:22
    artifacts: [dist]
    steps:
      - run: npm ci
      - run: npm run build
  ship:
    stage: deploy
    needs: [site]
    steps:
      - uses: deploy
`,
	},
	{
		ID: "docker-deploy", Name: "Dockerfile build and deploy", Language: "docker",
		Description: "Build the repository Dockerfile and deploy it, no test stage.",
		Tags:        []string{"build", "deploy"},
		YAML: `version: 1
name: docker-deploy
on:
  push:
    branches: [main]
  manual: {}
stages: [build, deploy]
jobs:
  build:
    stage: build
    steps:
      - uses: build
        id: image
  ship:
    stage: deploy
    steps:
      - uses: deploy
        with:
          strategy: blue-green
      - uses: notify
        with:
          message: deployed
`,
	},
	{
		ID: "monorepo-paths", Name: "Monorepo with path filters", Language: "monorepo",
		Description: "Test the api and web folders independently, only when their files change.",
		Tags:        []string{"test", "monorepo"},
		YAML: `version: 1
name: monorepo
on:
  push:
    branches: [main]
    paths: ["api/**", "web/**"]
  pull_request:
    paths: ["api/**", "web/**"]
jobs:
  api:
    image: golang:1.23
    steps:
      - run: cd api && go test ./...
  web:
    image: node:22
    steps:
      - run: cd web && npm ci
      - run: cd web && npm test --if-present
`,
	},
	{
		ID: "nightly-job", Name: "Nightly scheduled job", Language: "any",
		Description: "Run a script on a cron schedule and on demand, for cleanups and reports.",
		Tags:        []string{"cron", "schedule"},
		YAML: `version: 1
name: nightly
on:
  schedule: ["0 3 * * *"]
  manual: {}
jobs:
  nightly:
    image: alpine:3.20
    timeout: 30m
    steps:
      - run: echo "replace this step with your nightly task"
`,
	},
}
