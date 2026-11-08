package docker

// RuntimeOptionsLabelKey records a hash of the editable runtime options (tmpfs,
// ulimits, init, stop timeout, hosts, dns, sysctls, pids limit) a container was
// created with. Must stay under spec.ReservedLabelPrefix.
const RuntimeOptionsLabelKey = "platform-reserved.runtime-options"
