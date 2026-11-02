package main

import (
	"bytes"
	"strings"
	"testing"
)

const vercelPullSample = `# Created by Vercel CLI
DATABASE_URL="db-conn-value-hunter2pw"
EMPTY=""
PLAIN=plainvalue99
VERCEL_ENV="production"
VERCEL_URL="app-abc.vercel.app"
`

func TestVercelEnvPullImportSkipsPlatformKeysAndHidesValues(t *testing.T) {
	plan := planEnvImport(map[string]string{}, parseEnvFileBytes([]byte(vercelPullSample)), nil, false)
	if len(plan.New) != 3 || len(plan.Platform) != 2 {
		t.Fatalf("new=%v platform=%v", plan.New, plan.Platform)
	}
	if _, ok := plan.Merged["VERCEL_ENV"]; ok {
		t.Fatal("VERCEL_ keys must not be merged")
	}
	var out bytes.Buffer
	printEnvImportHuman(&out, "shop", envImportResult{App: "shop", DryRun: true, Plan: plan})
	for _, v := range []string{"hunter2pw", "plainvalue99", "app-abc.vercel.app"} {
		if strings.Contains(out.String(), v) {
			t.Fatalf("output leaks a value: %q", v)
		}
	}
	if !strings.Contains(out.String(), "VERCEL_ENV") {
		t.Fatal("dry run must list skipped keys")
	}
}
