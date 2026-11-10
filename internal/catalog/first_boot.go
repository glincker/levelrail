package catalog

// maxFirstBootSeconds bounds first_boot_seconds at submission; it mirrors the
// registry max of templates.first_boot_max_seconds without importing runtimesettings.
const maxFirstBootSeconds = 7200
