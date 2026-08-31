// Client-side provenance for an app's effective env, computed by diffing
// the three shared-env tiers against the app's own env: no dedicated
// backend endpoint returns "which tier last set this key" today, and
// this is read-only display logic, so diffing here is an acceptable
// first pass rather than a new write-path or a new combined endpoint.
//
// Mirrors internal/reconcile/application's resolveEnv override order
// exactly: organization is the base layer, then project, then
// environment, then the app's own env on top of all three.

export type EnvTier = 'organization' | 'project' | 'environment'

export interface EnvTierValue {
  tier: EnvTier
  value: string
}

export interface EnvTierLayers {
  organization?: Record<string, string>
  project?: Record<string, string>
  environment?: Record<string, string>
}

const TIER_ORDER: EnvTier[] = ['organization', 'project', 'environment']

// computeInheritedEnv returns, for every key any of the three tiers
// define, the highest-priority tier below the app's own env that
// defines it (and that tier's value). This is exactly the value that
// would apply if the app didn't set that key itself, whether or not it
// actually does.
export function computeInheritedEnv(
  layers: EnvTierLayers,
): Record<string, EnvTierValue> {
  const byTier: Record<EnvTier, Record<string, string> | undefined> = {
    organization: layers.organization,
    project: layers.project,
    environment: layers.environment,
  }
  const result: Record<string, EnvTierValue> = {}
  for (const tier of TIER_ORDER) {
    for (const [key, value] of Object.entries(byTier[tier] ?? {})) {
      result[key] = { tier, value }
    }
  }
  return result
}

// inheritedOnlyKeys returns computeInheritedEnv's entries restricted to
// keys the app does not set itself: the rows EnvEditor shows read-only,
// since they're part of the app's effective env but aren't part of its
// own editable/save-able env map.
export function inheritedOnlyEnv(
  layers: EnvTierLayers,
  ownEnv: Record<string, string> | undefined,
): Array<{ key: string; value: string; tier: EnvTier }> {
  const ownKeys = new Set(Object.keys(ownEnv ?? {}))
  return Object.entries(computeInheritedEnv(layers))
    .filter(([key]) => !ownKeys.has(key))
    .map(([key, info]) => ({ key, value: info.value, tier: info.tier }))
    .sort((a, b) => a.key.localeCompare(b.key))
}
