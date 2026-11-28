package compose

import "strings"

// keyReason says why an unread compose key has no effect and how much it matters.
type keyReason struct {
	reason string
	// changes is true when dropping the key alters how the service runs.
	changes bool
}

// keyReasons covers the keys pasted most often; the rest get a generic reason.
var keyReasons = map[string]keyReason{
	"container_name":                       {"containers are named <app>-<service>, so the custom name is not applied", false},
	"hostname":                             {"the container hostname is not set", false},
	"expose":                               {"internal ports need no declaration here", false},
	"stop_signal":                          {"the image's default stop signal is used", true},
	"tty":                                  {"containers run without a TTY", false},
	"stdin_open":                           {"containers run without stdin", false},
	"logging":                              {"the logging driver and options are not applied; the platform collects logs itself", false},
	"links":                                {"legacy links are not created; service names already resolve", false},
	"external_links":                       {"links to containers outside the app are not created", true},
	"scale":                                {"the replica count is not read from scale", true},
	"build":                                {"a pasted file cannot build images", true},
	"mem_reservation":                      {"soft memory reservations are not applied", false},
	"memswap_limit":                        {"the swap limit is not applied", true},
	"cpu_shares":                           {"relative CPU weight is not applied", false},
	"cpuset":                               {"CPU pinning is not applied", true},
	"extends":                              {"extended services from other files are not merged", true},
	"env_file":                             {"env files on disk are not read from a pasted file", true},
	"healthcheck.disable":                  {"the healthcheck disable flag is not read", true},
	"depends_on.restart":                   {"restart-on-dependency-change is not applied", false},
	"depends_on.required":                  {"every listed dependency is treated as required", true},
	"ports.host_ip":                        {"ports are published on all interfaces, not the stated host IP", true},
	"ports.name":                           {"port names are labels only", false},
	"ports.app_protocol":                   {"the application protocol hint is not used", false},
	"volumes.bind":                         {"bind propagation options are not applied", true},
	"volumes.tmpfs":                        {"tmpfs size and mode options are not applied", true},
	"volumes.consistency":                  {"the consistency hint only applies to Docker Desktop", false},
	"volumes.volume":                       {"per-volume options such as nocopy are not applied", false},
	"deploy.resources.reservations.cpus":   {"CPU reservations are not applied, only limits", false},
	"deploy.resources.reservations.memory": {"memory reservations are not applied, only limits", false},
	"oom_score_adj":                        {"the OOM score adjustment is not applied", true},
	"cgroup_parent":                        {"a custom cgroup parent is not applied", true},
	"mac_address":                          {"a fixed MAC address is not applied", true},
	"domainname":                           {"the container domain name is not set", false},
	"dns_search":                           {"custom DNS search domains are not applied", true},
	"group_add":                            {"supplementary groups are not added", true},
	"volumes_from":                         {"volumes from another container are not shared", true},
	"runtime":                              {"a custom container runtime is not used", true},
	"isolation":                            {"the isolation technology is not applied", false},
	"storage_opt":                          {"storage driver options are not applied", true},
}

// Reason is the plain sentence for why the key is dropped.
func (u UnsupportedKey) Reason() string {
	if r, ok := keyReasons[u.Key]; ok {
		return r.reason
	}
	return "the key is not read, so the setting has no effect on how the service runs"
}

// ChangesBehavior reports whether dropping the key alters the running service.
func (u UnsupportedKey) ChangesBehavior() bool {
	if r, ok := keyReasons[u.Key]; ok {
		return r.changes
	}
	return !strings.HasPrefix(u.Key, "x-")
}
