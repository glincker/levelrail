/** Keys under gitBuild:failureActions. */
export type FailureLabelKey =
  | 'buildSettings'
  | 'deployLog'
  | 'registry'
  | 'nodes'
  | 'buildPerformance'
  | 'health'
  | 'appLogs'
  | 'resources'
  | 'environment'
  | 'deployPolicies'
  | 'approvals'
  | 'security'
  | 'deploys'
  | 'source'

export interface FailureAction {
  href: string
  labelKey: FailureLabelKey
}

interface FailureTarget {
  path: (app: string, deployId: string) => string
  labelKey: FailureLabelKey
}

const app = (name: string) => `/apps/${encodeURIComponent(name)}`

const BUILD_SETTINGS: FailureTarget = {
  path: (a) => `${app(a)}/source#build-settings`,
  labelKey: 'buildSettings',
}
const DEPLOY_LOG: FailureTarget = {
  path: (a, d) => `${app(a)}/deploys/${encodeURIComponent(d)}/logs`,
  labelKey: 'deployLog',
}
const REGISTRY: FailureTarget = {
  path: () => '/settings/registry-credentials',
  labelKey: 'registry',
}
const SOURCE: FailureTarget = {
  path: (a) => `${app(a)}/source`,
  labelKey: 'source',
}
const NODES: FailureTarget = { path: () => '/nodes', labelKey: 'nodes' }

// Every code in internal/failure has a destination; unknown falls back to the log.
const TARGETS: Record<string, FailureTarget> = {
  dockerfile_error: BUILD_SETTINGS,
  build_config_error: BUILD_SETTINGS,
  dependency_install_failed: DEPLOY_LOG,
  compile_error: DEPLOY_LOG,
  lockfile_out_of_sync: DEPLOY_LOG,
  build_out_of_memory: {
    path: () => '/settings/build-performance',
    labelKey: 'buildPerformance',
  },
  build_timeout: {
    path: () => '/settings/build-performance',
    labelKey: 'buildPerformance',
  },
  image_pull_failed: REGISTRY,
  registry_auth_failed: REGISTRY,
  registry_push_failed: REGISTRY,
  platform_mismatch: NODES,
  disk_full: NODES,
  docker_unreachable: NODES,
  port_not_listening: {
    path: (a) => `${app(a)}/health`,
    labelKey: 'health',
  },
  health_check_failed: {
    path: (a) => `${app(a)}/health`,
    labelKey: 'health',
  },
  container_crashed: {
    path: (a) => `${app(a)}/logs`,
    labelKey: 'appLogs',
  },
  oom_killed: {
    path: (a) => `${app(a)}/resources`,
    labelKey: 'resources',
  },
  missing_env: {
    path: (a) => `${app(a)}/environment`,
    labelKey: 'environment',
  },
  freeze_window_hold: {
    path: () => '/settings/deploy-policies',
    labelKey: 'deployPolicies',
  },
  approval_pending: { path: () => '/approvals', labelKey: 'approvals' },
  scan_gate_blocked: { path: () => '/security', labelKey: 'security' },
  rollback_target_gone: {
    path: (a) => `${app(a)}/deploys`,
    labelKey: 'deploys',
  },
  git_host_unreachable: SOURCE,
  source_not_cached: SOURCE,
}

/** failureActionFor returns the deep link that fixes a failure code, never null for a known app. */
export function failureActionFor(
  code: string,
  appName: string,
  deployId: string,
): FailureAction | null {
  if (!appName) return null
  const target = TARGETS[code] ?? DEPLOY_LOG
  return { href: target.path(appName, deployId), labelKey: target.labelKey }
}

/** isUnknownFailure is true when no rule matched, so the log is the only lead. */
export function isUnknownFailure(code: string): boolean {
  return !(code in TARGETS)
}
