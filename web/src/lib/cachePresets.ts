import type { CacheRule, CacheSpec } from '../queries/domainPolicyTypes'

// Mirrors internal/trafficpolicy NextJSPreset; the server copy is the source
// of truth for the CLI and API.
export const NEXTJS_VARY = [
  'rsc',
  'next-router-state-tree',
  'next-router-prefetch',
  'next-router-segment-prefetch',
]

// The cache refuses to vary on these, and a wildcard cannot be keyed.
const UNVARIABLE = new Set(['cookie', 'authorization', '*'])

export function canVaryOn(header: string): boolean {
  return header !== '' && !UNVARIABLE.has(header.toLowerCase())
}

export function nextjsPresetRules(): CacheRule[] {
  const methods = ['GET', 'HEAD']
  return [
    {
      match: { kind: 'prefix', path: '/_next/static', methods },
      ttl_seconds: 86400,
      stale_while_revalidate_seconds: 60,
      stale_if_error_seconds: 86400,
    },
    {
      match: { kind: 'prefix', path: '/', methods },
      ttl_seconds: 60,
      stale_while_revalidate_seconds: 30,
      stale_if_error_seconds: 600,
      vary: [...NEXTJS_VARY],
    },
  ]
}

function mergeVary(have: string[] | undefined, add: string[]): string[] {
  const out = [...(have ?? [])]
  for (const h of add) {
    if (!out.some((x) => x.toLowerCase() === h.toLowerCase())) out.push(h)
  }
  return out
}

/** Adds the Next.js preset rules, skipping rules already present, and turns caching on. */
export function withNextjsPreset(spec: CacheSpec): CacheSpec {
  const rules = [...spec.rules]
  for (const add of nextjsPresetRules()) {
    const same = (r: CacheRule) =>
      r.match.kind === add.match.kind && r.match.path === add.match.path
    const i = rules.findIndex(same)
    if (i < 0) rules.push(add)
    else
      rules[i] = {
        ...rules[i]!,
        vary: mergeVary(rules[i]!.vary, add.vary ?? []),
      }
  }
  return { ...spec, enabled: true, rules }
}

/**
 * Adds headers to the Vary list of every cache rule. With no cache rule to
 * extend, falls back to the given rules so the fix still has an effect.
 */
export function withVary(
  spec: CacheSpec,
  headers: string[],
  fallback: CacheRule[],
): CacheSpec {
  const live = spec.rules.filter((r) => !r.never)
  if (live.length === 0) {
    if (fallback.length === 0) return spec
    return { ...spec, enabled: true, rules: [...spec.rules, ...fallback] }
  }
  return {
    ...spec,
    rules: spec.rules.map((r) =>
      r.never ? r : { ...r, vary: mergeVary(r.vary, headers) },
    ),
  }
}
