package platformimport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCapRoverHookNotes(t *testing.T) {
	raw := `{"appName":"web","preDeployFunction":"async function f(){ token='s3cr3t-value' }","nodeId":"n123",
"tags":[{"tagName":"blue"}],"appDeployTokenConfig":{"enabled":true,"appDeployToken":"tok-secret-value"}}`
	var def caproverDefinition
	if err := json.Unmarshal([]byte(raw), &def); err != nil {
		t.Fatal(err)
	}
	app := mapCapRoverApp(def, "example.test")
	var all []string
	for _, n := range app.Notes {
		all = append(all, n.Area+" "+n.Reason+" "+n.Manual)
	}
	got := strings.Join(all, "\n")
	for _, want := range []string{"pre-deploy script", "node", "blue", "deploy token"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in notes:\n%s", want, got)
		}
	}
	for _, leak := range []string{"s3cr3t-value", "tok-secret-value", "n123"} {
		if strings.Contains(got, leak) {
			t.Errorf("notes leak %q", leak)
		}
	}
}
