import type { DeployFailureFix } from '../types/deployFailure'
import type { DeployStage } from './deployStages'

export type DeployFixTarget =
  | '/apps/$name/environment'
  | '/apps/$name/source'
  | '/apps/$name/deploy-settings'
  | '/apps/$name/resources'

export interface DeployFixLink {
  to: DeployFixTarget
  /** Key under deployDetail `fix.link.*`. */
  labelKey: 'environment' | 'source' | 'deploySettings' | 'resources'
}

const ENV: DeployFixLink = {
  to: '/apps/$name/environment',
  labelKey: 'environment',
}
const SOURCE: DeployFixLink = { to: '/apps/$name/source', labelKey: 'source' }
const SETTINGS: DeployFixLink = {
  to: '/apps/$name/deploy-settings',
  labelKey: 'deploySettings',
}
const RESOURCES: DeployFixLink = {
  to: '/apps/$name/resources',
  labelKey: 'resources',
}

// Keyed by buildLogHints rule ids. Registry auth lives with the env vars
// because credentials are supplied as secrets.
const BY_CAUSE: Record<string, DeployFixLink[]> = {
  'npm-registry-auth': [ENV],
  'registry-auth': [ENV, SOURCE],
  'missing-env-var': [ENV],
  'npm-dependency-not-found': [SOURCE],
  'pip-dependency-not-found': [SOURCE],
  'dependency-install': [SOURCE],
  'dockerfile-path': [SOURCE],
  'image-not-found': [SOURCE],
  'port-mismatch': [SETTINGS],
  'health-check-timeout': [SETTINGS],
  'node-heap-oom': [RESOURCES],
  'go-runtime-oom': [RESOURCES],
  'oom-killed': [RESOURCES],
  'disk-space': [SOURCE],
  'permission-denied': [SOURCE],
}

// Without a matched cause, fall back to the page that owns the failing stage.
function byStage(stage: DeployStage['key']): DeployFixLink[] {
  return stage === 'build' ? [SOURCE] : [SETTINGS]
}

// Typed fix kinds from the control plane. 'rerun' needs no setting change.
const BY_FIX_KIND: Partial<Record<DeployFailureFix['kind'], DeployFixLink>> = {
  set_secret: ENV,
  edit_source: SOURCE,
  raise_memory: RESOURCES,
  free_disk: SOURCE,
}

export function deployFixLinks(
  stage: DeployStage['key'],
  causeId?: string,
  fixes?: DeployFailureFix[],
): DeployFixLink[] {
  const typed = [
    ...new Set(
      (fixes ?? [])
        .map((f) => BY_FIX_KIND[f.kind])
        .filter((l): l is DeployFixLink => l !== undefined),
    ),
  ]
  if (typed.length > 0) return typed
  const matched = causeId ? BY_CAUSE[causeId] : undefined
  return matched ?? byStage(stage)
}
