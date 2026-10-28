package compose

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// NoticeLevel distinguishes a purely informational Notice from an
// operator-facing Warning about a semantic gap. Neither ever fails
// Validate: both describe compose keywords that parsed successfully but
// don't behave the way they would under real Docker Compose.
type NoticeLevel string

// The two NoticeLevel values Notices ever produces.
const (
	NoticeLevelNote    NoticeLevel = "note"
	NoticeLevelWarning NoticeLevel = "warning"
)

// Notice is one non-blocking observation about a parsed compose file.
type Notice struct {
	Level   NoticeLevel
	Message string
	// Service, Key and Alternative are set for an ignored-key warning.
	Service     string
	Key         string
	Alternative string
}

// Notices reports every non-blocking observation across f:
//
//   - restart: (NoticeLevelNote): an explicit "no" or "on-failure[:N]"
//     runs the service to completion (exit 0 is done, not restarted);
//     an unrecognized value is warned about and kept running.
//   - networks: (NoticeLevelWarning): every service in an app already
//     shares one flat Docker network (internal/reconcile/application's
//     NetworkName, one per app, not per compose network), so a compose
//     file's custom networks: can't isolate services from each other
//     here the way they would under real Compose. That's a real
//     semantic gap for a file that expressed isolation intent, worth a
//     warning rather than a silent drop.
//   - depends_on: (NoticeLevelNote): parses into DependsOn and IS
//     enforced as start order; condition: service_healthy additionally
//     waits for the dependency's readiness condition.
func (f *File) Notices() []Notice {
	var notices []Notice
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if n, ok := restartNotice(name, svc.Restart); ok {
			notices = append(notices, n)
		}
	}
	if f.declaresCustomNetworks() {
		notices = append(notices, Notice{
			Level:   NoticeLevelWarning,
			Message: "networks: is declared but not enforced; every service in this app shares one network and can already reach every other service, regardless of any networks: assignment",
		})
	}
	if f.declaresDependsOn() {
		notices = append(notices, Notice{
			Level:   NoticeLevelNote,
			Message: "depends_on: is enforced as start order (a dependency's container must be running before its dependent is created); add condition: service_healthy to also wait for the dependency's readiness check",
		})
	}
	for _, u := range f.Unsupported {
		notices = append(notices, Notice{Level: NoticeLevelWarning, Message: u.Message(), Service: u.Service, Key: u.Key, Alternative: u.Alternative})
	}
	return notices
}

func (f *File) declaresCustomNetworks() bool {
	if len(f.Networks) > 0 {
		return true
	}
	for _, name := range sortedServiceNames(f) {
		if len(f.Services[name].Networks) > 0 {
			return true
		}
	}
	return false
}

// declaresDependsOn reports a depends_on entry that only waits for start;
// a file that already uses condition: service_healthy everywhere needs no note.
func (f *File) declaresDependsOn() bool {
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if len(svc.DependsOn) > len(svc.DependsOnHealthy) {
			return true
		}
	}
	return false
}

// restartNotice explains what an explicit restart: value does here. always and
// unless-stopped match the platform's own keep-running default, so they need none.
func restartNotice(service, raw string) (Notice, bool) {
	policy := normalizeRestart(raw)
	switch {
	case policy == "" || policy == "always" || policy == "unless-stopped":
		return Notice{}, false
	case store.IsRunToCompletionRestart(policy):
		return Notice{Level: NoticeLevelNote, Message: fmt.Sprintf("service %q: restart: %q runs it to completion: exit code 0 marks it done and it is not restarted; a nonzero exit is restarted with crash backoff", service, raw)}, true
	default:
		return Notice{Level: NoticeLevelWarning, Message: fmt.Sprintf("service %q: restart: %q is not a recognized policy; the service is kept running like restart: always", service, raw)}, true
	}
}
