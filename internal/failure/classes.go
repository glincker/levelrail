package failure

// Failure codes.
const (
	CodeDockerfileError    = "dockerfile_error"
	CodeDependencyInstall  = "dependency_install_failed"
	CodeCompileError       = "compile_error"
	CodeBuildConfigError   = "build_config_error"
	CodeBuildOOM           = "build_out_of_memory"
	CodeBuildTimeout       = "build_timeout"
	CodeImagePullFailed    = "image_pull_failed"
	CodePortNotListening   = "port_not_listening"
	CodeHealthCheckFailed  = "health_check_failed"
	CodeContainerCrashed   = "container_crashed"
	CodeOOMKilled          = "oom_killed"
	CodeMissingEnv         = "missing_env"
	CodeRegistryPushFailed = "registry_push_failed"
	CodeDiskFull           = "disk_full"
	CodeFreezeWindow       = "freeze_window_hold"
	CodeApprovalPending    = "approval_pending"
	CodeScanGateBlocked    = "scan_gate_blocked"
	CodeRollbackTargetGone = "rollback_target_gone"
	CodeDockerUnreachable  = "docker_unreachable"
	CodeUnknown            = "unknown"

	docsPagePath = "/deploy-failures"
)

func docsURL(code string) string { return docsPagePath + "#" + code }

type class struct {
	code        string
	cause       string
	fix         string
	retryable   bool
	patterns    []string
	buildOnly   bool
	runtimeOnly bool
	held        bool
}

// classes is checked in order, most specific first. Patterns are lowercase
// substrings.
var classes = []class{
	{
		code: CodeFreezeWindow, held: true, retryable: true,
		cause:    "The deploy is held by a freeze window and will not start until it ends.",
		fix:      "Wait for the freeze window to end (the held deploy is released automatically), or redeploy with an explicit freeze override and a reason.",
		patterns: []string{"frozen", "freeze"},
	},
	{
		code: CodeApprovalPending, held: true, retryable: true,
		cause:    "The deploy is waiting for an approval before it can proceed.",
		fix:      "Ask an approver to approve or reject the pending deploy approval, then the deploy continues.",
		patterns: []string{"approval", "awaiting", "held"},
	},
	{
		code:     CodeRollbackTargetGone,
		cause:    "The image this rollback targets was garbage collected, so it cannot be redeployed.",
		fix:      "Redeploy from source instead, and raise the number of retained rollback images so older releases stay available.",
		patterns: []string{"garbage collected", "can no longer be rolled back"},
	},
	{
		code:     CodeScanGateBlocked,
		cause:    "The supply chain scan gate blocked this release because the image has findings above the configured threshold.",
		fix:      "Fix or upgrade the vulnerable packages and redeploy, or have an operator with override rights record a reasoned override.",
		patterns: []string{"supply chain gate blocked"},
	},
	{
		code:     CodeMissingEnv,
		cause:    "A required environment variable or secret has no value.",
		fix:      "Set the missing variable or secret on the app (the error names it), then redeploy.",
		patterns: []string{"is required but no secret value", "missing required env", "required environment variable", "references database", "vault secret, but vault integration"},
	},
	{
		code:     CodeDiskFull,
		cause:    "The host ran out of disk space.",
		fix:      "Free disk on the node (prune unused images and build cache, remove old logs), then retry.",
		patterns: []string{"no space left on device", "disk quota exceeded", "insufficient disk space"},
	},
	{
		code: CodeDockerUnreachable, retryable: true,
		cause:    "The control plane could not reach the Docker daemon on the target node.",
		fix:      "Confirm the Docker daemon is running and reachable on the node, then retry the deploy.",
		patterns: []string{"cannot connect to the docker daemon", "the docker daemon is not running", "dial unix /var/run/docker.sock"},
	},
	{
		code: CodeBuildOOM, buildOnly: true,
		cause:    "The build ran out of memory and was killed.",
		fix:      "Give the build node more memory, reduce parallelism in the build (for example lower the bundler worker count), or build a smaller target.",
		patterns: []string{"oomkilled", "out of memory", "signal: killed", "exit code: 137", "exit code 137", "exit status 137", "heap out of memory"},
	},
	{
		code: CodeOOMKilled, runtimeOnly: true,
		cause:    "The container was killed for exceeding its memory limit.",
		fix:      "Raise the app's memory limit or reduce its memory use, then redeploy.",
		patterns: []string{"oomkilledduringreadiness", "oomkilled", "out of memory", "exit code 137", "exit status 137"},
	},
	{
		code: CodeRegistryPushFailed, retryable: true,
		cause:    "Pushing the built image to the registry failed.",
		fix:      "Check the registry credential and repository permissions, and that the registry is reachable from the build node, then retry.",
		patterns: []string{"failed to push", "error pushing", "push access denied", "blob upload unknown", "pushing to registry"},
	},
	{
		code:     CodeImagePullFailed,
		cause:    "The image could not be pulled: it does not exist, the tag is wrong, or the registry denied access.",
		fix:      "Check the image name and tag, and add or refresh the registry credential if the registry is private.",
		patterns: []string{"pull access denied", "manifest unknown", "no such image", "repository does not exist", "unauthorized: authentication required", "toomanyrequests", "requested access to the resource is denied", "not found: manifest", "failed to resolve source metadata"},
	},
	{
		code:     CodeDockerfileError,
		cause:    "The Dockerfile is invalid or a file it references is missing from the build context.",
		fix:      "Check build.path and build.baseDirectory in the app spec, fix the Dockerfile syntax, and make sure every COPY source is committed and not excluded by .dockerignore. A repo with no Dockerfile builds with build.type railpack (--build-type railpack), which a manual rebuild does not remember.",
		patterns: []string{"failed to read dockerfile", "dockerfile parse error", "unknown instruction", "cannot locate dockerfile", "unable to prepare context", "failed to compute cache key", "failed to calculate checksum", "dockerfile: no such file", "no such file or directory: dockerfile", "copy failed: file not found"},
	},
	{
		code: CodeBuildTimeout, buildOnly: true, retryable: true,
		cause:    "The build ran past its time limit.",
		fix:      "Enable the remote build cache, split slow stages, or raise the build timeout, then retry.",
		patterns: []string{"context deadline exceeded", "build timed out", "deadline exceeded"},
	},
	{
		code: CodeBuildConfigError, buildOnly: true,
		cause:    "The build could not work out what to build or run: a script the build calls is missing, or no start command was detected.",
		fix:      "Add the missing script to package.json, or set an explicit start command (or a Dockerfile) in the app spec, then redeploy.",
		patterns: []string{"missing script:", "no start command", "failed to generate build plan", "could not determine how to build"},
	},
	{
		code: CodeCompileError, buildOnly: true,
		cause:    "The application code failed to compile during the build.",
		fix:      "Fix the compiler error shown in the log excerpt (the file and line are in the decisive line), confirm it builds locally, then redeploy.",
		patterns: []string{"error ts", "failed to compile", ": undefined: ", "could not compile", "error[e", "cannot find symbol", "compilation failure", "compilation error", "type error:", "build error occurred"},
	},
	{
		code: CodeDependencyInstall, buildOnly: true,
		cause:    "A dependency install step in the build failed.",
		fix:      "Check the failing install command in the log excerpt: fix the lockfile or version pin, and confirm the package registry is reachable from the build node.",
		patterns: []string{"npm err!", "npm error", "eresolve", "could not resolve host", "unable to locate package", "no matching distribution found", "could not find a version that satisfies", "err_pnpm", "yarn error", "error an unexpected error occurred", "there appears to be trouble with your network", "eai_again", "e: failed to fetch", "temporary failure resolving", "pip install", "process \"/bin/sh"},
	},
	{
		code:     CodePortNotListening,
		cause:    "The container started but nothing is listening on the configured port.",
		fix:      "Make the app listen on 0.0.0.0 at the port set in the app spec (the PORT env var is injected), or change the spec port to match what the app binds.",
		patterns: []string{"connection refused", "port is already allocated", "address already in use", "not listening"},
	},
	{
		code: CodeHealthCheckFailed, runtimeOnly: true,
		cause:    "The container started but never passed its readiness health check.",
		fix:      "Confirm health.readiness.path returns a success status quickly, or raise health.readyTimeout if the app just starts slowly.",
		patterns: []string{"readinessfailed", "readiness probe", "timed out waiting for", "health check"},
	},
	{
		code: CodeContainerCrashed, runtimeOnly: true,
		cause:    "The container exited or is restarting repeatedly.",
		fix:      "Read the log excerpt for what happens right after start: a bad command, a missing file or a config error is the usual cause. Fix it and redeploy, or roll back.",
		patterns: []string{"exitedduringreadiness", "exited with code", "exit code", "startfailed", "createfailed", "crashloop"},
	},
}

func classByCode(code string) *class {
	for i := range classes {
		if classes[i].code == code {
			return &classes[i]
		}
	}
	return nil
}
