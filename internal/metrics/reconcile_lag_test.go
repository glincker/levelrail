package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestWriteReconcileLag(t *testing.T) {
	var b bytes.Buffer
	err := WriteReconcileLag(&b, "lr_", map[string]time.Duration{"mesh": 1500 * time.Millisecond, "app": 0})
	if err != nil {
		t.Fatal(err)
	}
	want := "# HELP lr_reconcile_controller_lag_seconds Age of the oldest desired change a reconciler has not yet observed, seconds.\n" +
		"# TYPE lr_reconcile_controller_lag_seconds gauge\n" +
		"lr_reconcile_controller_lag_seconds{reconciler=\"app\"} 0\n" +
		"lr_reconcile_controller_lag_seconds{reconciler=\"mesh\"} 1.5\n"
	if b.String() != want {
		t.Fatalf("got:\n%s", strings.ReplaceAll(b.String(), "\n", "\\n\n"))
	}
}
