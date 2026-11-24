package metrics

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ReconcileLagName is the gauge for the oldest desired change not yet observed
// by a successful reconcile, one series per reconciler. The prefix is the caller's.
const ReconcileLagName = "reconcile_controller_lag_seconds"

// WriteReconcileLag renders the lag gauge in the Prometheus text format, with
// the series sorted so scrapes are stable.
func WriteReconcileLag(w io.Writer, prefix string, lag map[string]time.Duration) error {
	name := prefix + ReconcileLagName
	if _, err := fmt.Fprintf(w, "# HELP %s Age of the oldest desired change a reconciler has not yet observed, seconds.\n# TYPE %s gauge\n", name, name); err != nil {
		return err
	}
	kinds := make([]string, 0, len(lag))
	for k := range lag {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	esc := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	for _, k := range kinds {
		v := strconv.FormatFloat(lag[k].Seconds(), 'f', -1, 64)
		if _, err := fmt.Fprintf(w, "%s{reconciler=\"%s\"} %s\n", name, esc.Replace(k), v); err != nil {
			return err
		}
	}
	return nil
}
