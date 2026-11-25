package main

import (
	"encoding/json"
	"io"
	"testing"
)

func TestSchemaDescribesACommand(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"schema", "docs"})
	var doc cliSchemaDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(doc.GlobalFlags) == 0 || len(doc.Commands) != 1 || doc.Commands[0].Command != "docs" {
		t.Fatalf("unexpected schema: %+v", doc)
	}
	if code := run("levelrail-cli-test", []string{"schema", "no-such-command"}, io.Discard, io.Discard, envMap()); code != exitUsage {
		t.Fatalf("unknown command exit = %d", code)
	}
}
