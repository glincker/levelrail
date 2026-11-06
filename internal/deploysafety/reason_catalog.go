package deploysafety

import (
	"fmt"
	"sort"
	"strings"
)

// ReasonInfo explains one status reason in plain words. Every Reason*
// constant needs an entry; a drift test fails the build when one is missing.
type ReasonInfo struct {
	Code     string `json:"code"`
	Kind     string `json:"kind"`
	Meaning  string `json:"meaning"`
	NextStep string `json:"next_step"`
}

// Kinds group reasons by where in a deploy they happen.
const (
	ReasonKindGate     = "gate"
	ReasonKindWatch    = "watch"
	ReasonKindRollback = "rollback"
	ReasonKindQueue    = "queue"
)

var reasonCatalog = []ReasonInfo{
	{ReasonReadinessFailed, ReasonKindGate, "The new container never passed its readiness probe. The old release keeps serving when the strategy allows it.", "Check the probe path and port, then read the app logs and redeploy."},
	{ReasonOOMKilledDuringReadiness, ReasonKindGate, "The new container was killed for running out of memory before it became ready.", "Raise the app memory limit or reduce start-up memory use, then redeploy."},
	{ReasonExitedDuringReadiness, ReasonKindGate, "The new container exited before it became ready.", "Read the app logs for the start-up error, fix it and redeploy."},
	{ReasonOOMKilledDuringWindow, ReasonKindGate, "The new container was ready but was killed for memory during the stability window.", "Raise the app memory limit, then redeploy."},
	{ReasonExitedDuringWindow, ReasonKindGate, "The new container was ready but exited during the stability window.", "Read the app logs for the crash cause, fix it and redeploy."},
	{ReasonRestartedDuringWindow, ReasonKindGate, "The new container restarted during the stability window, so it was not trusted with traffic.", "Read the app logs for the restart cause, fix it and redeploy."},
	{ReasonGateTimeout, ReasonKindGate, "The stability gate did not finish inside its time limit.", "Check that the app starts promptly and the probe responds, then redeploy."},
	{ReasonWatchCrashloop, ReasonKindWatch, "The live release started crash looping after cutover.", "Read the app logs, fix the crash or roll back to the previous release."},
	{ReasonWatchOOMKilled, ReasonKindWatch, "The live release was killed for memory after cutover.", "Raise the app memory limit or roll back to the previous release."},
	{ReasonWatchExited, ReasonKindWatch, "The live release exited after cutover.", "Read the app logs for the exit cause or roll back to the previous release."},
	{ReasonWatchErrorRate, ReasonKindWatch, "The error rate of the live release crossed the post-deploy watch limit.", "Check the app logs and recent changes, fix forward or roll back."},
	{ReasonWatchLatency, ReasonKindWatch, "Response time of the live release regressed past the post-deploy watch limit.", "Compare latency with the previous release, fix forward or roll back."},
	{ReasonWatchStopped, ReasonKindWatch, "An operator ended the post-deploy watch early.", "Nothing to fix. Redeploy or restart the watch if you still want the safety net."},
	{ReasonAutoRolledBack, ReasonKindRollback, "The post-deploy watch tripped and the previous release was restored automatically.", "Find out why the new release tripped the watch before deploying it again."},
	{ReasonNoRollbackTarget, ReasonKindRollback, "The watch tripped but there was no earlier healthy release to go back to.", "Fix the app and redeploy, or deploy a known good image by hand."},
	{ReasonOperatorRollback, ReasonKindRollback, "An operator rolled the app back to an earlier release.", "Nothing to fix. Deploy again when the problem with the newer release is solved."},
	{"Frozen", ReasonKindQueue, "The deploy was held because a deploy freeze is active for this app.", "Wait for the freeze to end, or approve the held deploy if you are allowed to override it."},
	{"Superseded", ReasonKindQueue, "A newer deploy replaced this one before it went live.", "Nothing to fix. The newer deploy carries the latest change."},
	{"StaleCommit", ReasonKindQueue, "The commit is older than what is already deployed, so it was skipped.", "Nothing to fix. Push a new commit to move forward from here."},
	{"DetectedByPoll", ReasonKindQueue, "The deploy was started by the git poller because a webhook was missed.", "Nothing to fix. Check git webhook delivery if this happens often."},
	{"Canceled", ReasonKindQueue, "An operator canceled the deploy before it finished.", "Nothing to fix. Deploy again when you are ready."},
}

// ReasonCatalog returns every status reason with its explanation, sorted by
// code. The slice is a copy, so callers cannot change the catalog.
func ReasonCatalog() []ReasonInfo {
	out := append([]ReasonInfo(nil), reasonCatalog...)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// LookupReason finds one reason by its code.
func LookupReason(code string) (ReasonInfo, bool) {
	for _, r := range reasonCatalog {
		if r.Code == code {
			return r, true
		}
	}
	return ReasonInfo{}, false
}

// RenderReasonsMarkdown is the body of docs/status-reasons.md, generated so
// the page cannot drift from the catalog.
func RenderReasonsMarkdown() string {
	var b strings.Builder
	b.WriteString("---\ntitle: \"Status reasons\"\n")
	b.WriteString("description: Every reason a deploy can fail its gate or be rolled back, what it means and the next step.\n")
	b.WriteString("---\n\n")
	b.WriteString("{/* Generated by `go run ./scripts/gen-status-reasons`. Do not edit by hand. */}\n\n")
	b.WriteString("Each reason below appears in `apps deploys list` and in the deploy gate timeline. List them with the `status-reasons` CLI command or `GET /api/v1/status-reasons`.\n\n")
	b.WriteString("| Reason | Stage | What it means | Next step |\n| --- | --- | --- | --- |\n")
	for _, r := range ReasonCatalog() {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", r.Code, r.Kind, r.Meaning, r.NextStep)
	}
	return b.String()
}
