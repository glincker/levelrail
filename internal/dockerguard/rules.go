package dockerguard

import "net/http"

// Rule ids. They appear in audit rows, logs and the 403 body, so they are
// part of the public contract: never rename one, add a new id instead.
const (
	RuleEndpointNotAllowed  = "endpoint_not_allowed"
	RulePathNoncanonical    = "path_noncanonical"
	RuleBodyTooLarge        = "body_too_large"
	RuleBodyInvalid         = "body_invalid"
	RulePrivileged          = "privileged"
	RuleHostPID             = "host_pid"
	RuleHostIPC             = "host_ipc"
	RuleHostUTS             = "host_uts"
	RuleHostUserns          = "host_userns"
	RuleHostCgroupns        = "host_cgroupns"
	RuleNetworkHost         = "network_host"
	RuleCapAdd              = "cap_add_disallowed"
	RuleDevices             = "devices_not_granted"
	RuleDeviceCgroupRules   = "device_cgroup_rules"
	RuleSecurityOpt         = "security_opt_weakened"
	RuleMaskedPaths         = "masked_paths_override"
	RuleBindSensitive       = "bind_sensitive_path"
	RuleBindUndeclared      = "bind_undeclared"
	RuleMountType           = "mount_type_disallowed"
	RuleVolumesFrom         = "volumes_from"
	RuleVolumeBindDriverOpt = "volume_bind_driver_opts"
	RuleExecPrivileged      = "exec_privileged"
	RuleImageImport         = "image_import"
)

// AllRules lists every rule id, in the order docs/docker-access.md documents them.
var AllRules = []string{
	RuleEndpointNotAllowed, RulePathNoncanonical, RuleBodyTooLarge, RuleBodyInvalid,
	RulePrivileged, RuleHostPID, RuleHostIPC, RuleHostUTS, RuleHostUserns, RuleHostCgroupns,
	RuleNetworkHost, RuleCapAdd, RuleDevices, RuleDeviceCgroupRules, RuleSecurityOpt,
	RuleMaskedPaths, RuleBindSensitive, RuleBindUndeclared, RuleMountType, RuleVolumesFrom,
	RuleVolumeBindDriverOpt, RuleExecPrivileged, RuleImageImport,
}

// bodyKind names the validator an endpoint's request body goes through.
type bodyKind int

const (
	bodyNone bodyKind = iota
	bodyContainerCreate
	bodyContainerUpdate
	bodyExecCreate
	bodyVolumeCreate
	bodyImageCreate
)

// endpoint is one allowlisted (method, path pattern) pair.
type endpoint struct {
	Method  string
	Pattern string
	Body    bodyKind
}

// allowlist is every Engine API call internal/docker, internal/build and
// BuildKit make. Anything else is denied. docs/docker-access.md explains each.
var allowlist = []endpoint{
	{http.MethodGet, "/_ping", bodyNone},
	{http.MethodHead, "/_ping", bodyNone},
	{http.MethodGet, "/version", bodyNone},
	{http.MethodGet, "/info", bodyNone},
	{http.MethodGet, "/events", bodyNone},
	{http.MethodGet, "/system/df", bodyNone},
	{http.MethodPost, "/auth", bodyNone},

	{http.MethodGet, "/containers/json", bodyNone},
	{http.MethodPost, "/containers/create", bodyContainerCreate},
	{http.MethodGet, "/containers/*/json", bodyNone},
	{http.MethodPost, "/containers/*/start", bodyNone},
	{http.MethodPost, "/containers/*/stop", bodyNone},
	{http.MethodPost, "/containers/*/kill", bodyNone},
	{http.MethodPost, "/containers/*/wait", bodyNone},
	{http.MethodPost, "/containers/*/update", bodyContainerUpdate},
	{http.MethodDelete, "/containers/*", bodyNone},
	{http.MethodGet, "/containers/*/logs", bodyNone},
	{http.MethodGet, "/containers/*/stats", bodyNone},
	{http.MethodGet, "/containers/*/archive", bodyNone},
	{http.MethodHead, "/containers/*/archive", bodyNone},
	{http.MethodPut, "/containers/*/archive", bodyNone},
	{http.MethodPost, "/containers/*/exec", bodyExecCreate},
	{http.MethodPost, "/exec/*/start", bodyNone},
	{http.MethodPost, "/exec/*/resize", bodyNone},
	{http.MethodGet, "/exec/*/json", bodyNone},

	{http.MethodGet, "/images/json", bodyNone},
	{http.MethodPost, "/images/create", bodyImageCreate},
	{http.MethodPost, "/images/load", bodyNone},
	{http.MethodGet, "/images/get", bodyNone},
	{http.MethodPost, "/images/prune", bodyNone},
	{http.MethodGet, "/images/**/json", bodyNone},
	{http.MethodGet, "/images/**/get", bodyNone},
	{http.MethodPost, "/images/**/tag", bodyNone},
	{http.MethodDelete, "/images/**", bodyNone},
	{http.MethodGet, "/distribution/**/json", bodyNone},
	{http.MethodPost, "/build/prune", bodyNone},
	{http.MethodPost, "/grpc", bodyNone},
	{http.MethodPost, "/session", bodyNone},

	{http.MethodGet, "/networks", bodyNone},
	{http.MethodPost, "/networks/create", bodyNone},
	{http.MethodGet, "/networks/*", bodyNone},
	{http.MethodPost, "/networks/*/connect", bodyNone},
	{http.MethodPost, "/networks/*/disconnect", bodyNone},
	{http.MethodDelete, "/networks/*", bodyNone},

	{http.MethodGet, "/volumes", bodyNone},
	{http.MethodPost, "/volumes/create", bodyVolumeCreate},
	{http.MethodGet, "/volumes/*", bodyNone},
	{http.MethodDelete, "/volumes/*", bodyNone},
}

// matchEndpoint returns the first allowlisted endpoint for method and the
// canonical versionless path.
func matchEndpoint(method, path string) (endpoint, bool) {
	for _, e := range allowlist {
		if e.Method == method && matchPattern(e.Pattern, path) {
			return e, true
		}
	}
	return endpoint{}, false
}
