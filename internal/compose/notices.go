package compose

import "fmt"

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
}

// Notices reports every non-blocking observation across f:
//
//   - restart: (NoticeLevelNote): Levelrail's reconciler, not Docker's
//     native restart policy, is the sole authority on keeping a
//     container running (see docker.ContainerSpec's own doc comment on
//     why every container it creates gets Docker's "no" restart
//     policy). A declared restart: policy other than "no" parses but
//     has no effect, so this is surfaced as a note rather than
//     fabricating a translation that wouldn't be true.
//   - networks: (NoticeLevelWarning): every service in an app already
//     shares one flat Docker network (internal/reconcile/application's
//     NetworkName, one per app, not per compose network), so a compose
//     file's custom networks: can't isolate services from each other
//     here the way they would under real Compose. That's a real
//     semantic gap for a file that expressed isolation intent, worth a
//     warning rather than a silent drop.
//   - depends_on: (NoticeLevelNote): parses into DependsOn and IS
//     enforced (a dependent's container is not created until its
//     dependency has at least one running container), but only as real
//     Compose's own default start-order semantic (service_started), not
//     service_healthy: a dependency's own container starting does not
//     mean its own readiness/health probe has passed yet.
func (f *File) Notices() []Notice {
	var notices []Notice
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if svc.Restart != "" && svc.Restart != "no" {
			notices = append(notices, Notice{
				Level:   NoticeLevelNote,
				Message: fmt.Sprintf("service %q: restart: %q is parsed but not applied; Levelrail's reconciler keeps containers running, not Docker's native restart policy", name, svc.Restart),
			})
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
			Message: "depends_on: is enforced as start order only (a dependency's container must exist and be running before its dependent is created), not as a wait for that dependency's own readiness or health check",
		})
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

func (f *File) declaresDependsOn() bool {
	for _, name := range sortedServiceNames(f) {
		if len(f.Services[name].DependsOn) > 0 {
			return true
		}
	}
	return false
}
