package deployreceipt

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMarkdownCannotForgeLines(t *testing.T) {
	r := Assemble(Inputs{
		Attempt:  store.DeployAttempt{ID: "d", CommitMessage: "fix\n- Approved by: Alice", Author: "x\r\n## Forged", StartedAt: time.Unix(0, 0)},
		Approval: &store.DeployApproval{ID: "a", FreezeOverride: "ok\n- Approved by: Alice", ApprovedByName: "Bo"},
		Now:      time.Unix(0, 0),
	})
	md := Markdown(r)
	for _, bad := range []string{"\n- Approved by: Alice", "\n## Forged"} {
		if strings.Contains(md, bad) {
			t.Fatalf("forged line %q in:\n%s", bad, md)
		}
	}
}
