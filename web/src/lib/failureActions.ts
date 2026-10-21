/** Keys under gitBuild:failureActions. */
export type FailureLabelKey =
  | 'buildSettings'
  | 'deployLog'
  | 'registry'
  | 'nodes'
  | 'buildPerformance'
  | 'raiseBuildLimit'
  | 'health'
  | 'appLogs'
  | 'resources'
  | 'environment'
  | 'deployPolicies'
  | 'approvals'
  | 'security'
  | 'deploys'
  | 'source'
  | 'freeSpace'

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
const FREE_SPACE: FailureTarget = {
  path: () => '/settings/system-status#disk-resilience',
  labelKey: 'freeSpace',
}

// Lands on the app's build limits card, where one button raises the limit that was hit.
const RAISE_BUILD_LIMIT: FailureTarget = {
  path: (a) => `${app(a)}/deploys#build-limits`,
  labelKey: 'raiseBuildLimit',
}

// Every code in internal/failure has a destination; unknown falls back to the log.
const TARGETS: Record<string, FailureTarget> = {
  dockerfile_error: BUILD_SETTINGS,
  build_config_error: BUILD_SETTINGS,
  dependency_install_failed: DEPLOY_LOG,
  compile_error: DEPLOY_LOG,
  lockfile_out_of_sync: DEPLOY_LOG,
  build_out_of_memory: RAISE_BUILD_LIMIT,
  build_timeout: RAISE_BUILD_LIMIT,
  image_pull_failed: REGISTRY,
  registry_auth_failed: REGISTRY,
  registry_push_failed: REGISTRY,
  platform_mismatch: NODES,
  disk_full: FREE_SPACE,
  image_corrupt: FREE_SPACE,
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

const FIX_TARGETS: Record<string, FailureTarget> = {
  build_settings: BUILD_SETTINGS,
  build_performance: RAISE_BUILD_LIMIT,
  node_disk: FREE_SPACE,
  app_secrets: {
    path: (a) => `${app(a)}/environment`,
    labelKey: 'environment',
  },
}

/** fixActionFor links a build-log fix to the page where it is applied; null when the fix has no page (a retry). */
export function fixActionFor(
  target: string | undefined,
  appName: string,
): FailureAction | null {
  const t = target ? FIX_TARGETS[target] : undefined
  if (!t || !appName) return null
  return { href: t.path(appName, ''), labelKey: t.labelKey }
}
