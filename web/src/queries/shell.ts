// GET /api/v1/shell (internal/api/shell_overview.go): one round trip holding
// the bodies the shell would otherwise fetch one by one before first paint.
// Each body is seeded under its own query key; the per-endpoint queries keep
// refetching on their own schedules afterwards.

import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { brandQueryOptions } from './brand'
import { demoQueryOptions } from './demo'
import { devModeQueryOptions } from './devMode'
import { sessionQueryOptions } from './security'
import { preferencesQueryOptions } from './accountSelf'
import { accountAppearanceQueryOptions } from './appearanceSync'
import { localModeQueryOptions } from './localMode'
import { systemStatusQueryOptions } from './systemStatus'
import { signInRequestsQueryOptions } from './signIn'
import { projectListQueryOptions } from './projects'
import { teamListQueryOptions } from './teams'
import { setDemoInfo, type DemoInfo } from '../lib/demoStore'

export interface ShellOverview {
  sections: Record<string, unknown>
}

// Section name on the wire to the query key the shell reads it from.
export function shellSectionKeys() {
  return {
    brand: brandQueryOptions().queryKey,
    demo: demoQueryOptions().queryKey,
    dev_mode: devModeQueryOptions().queryKey,
    session: sessionQueryOptions().queryKey,
    preferences: preferencesQueryOptions().queryKey,
    appearance: accountAppearanceQueryOptions().queryKey,
    local_mode: localModeQueryOptions().queryKey,
    system_status: systemStatusQueryOptions().queryKey,
    sign_in_requests: signInRequestsQueryOptions().queryKey,
    projects: projectListQueryOptions().queryKey,
    teams: teamListQueryOptions().queryKey,
  }
}

// Seeds only sections the server returned; an omitted one (no permission,
// or it failed) is left for its own query to fetch and report as usual.
export function seedShellCache(qc: QueryClient, body: ShellOverview): number {
  const keys: Partial<Record<string, QueryKey>> = shellSectionKeys()
  let seeded = 0
  for (const [name, data] of Object.entries(body.sections ?? {})) {
    const key = keys[name]
    if (!key || data === undefined || qc.getQueryData(key) !== undefined)
      continue
    qc.setQueryData(
      key,
      name === 'appearance' ? appearanceFromBody(data) : data,
    )
    if (name === 'demo') setDemoInfo(data as DemoInfo)
    seeded++
  }
  return seeded
}

// Mirrors fetchAccountAppearance, which wraps the stored value on a 200.
function appearanceFromBody(data: unknown): {
  available: true
  value: unknown
} {
  const body = data as { value?: unknown } | null
  return { available: true, value: body?.value ?? null }
}

const DEFAULT_SHELL_SEED_TIMEOUT_MS = 1500

// How long first paint may wait on the aggregate; VITE_SHELL_SEED_TIMEOUT_MS overrides.
export function shellSeedTimeoutMs(
  env: Record<string, unknown> = import.meta.env,
): number {
  const raw = Number(env.VITE_SHELL_SEED_TIMEOUT_MS)
  return Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_SHELL_SEED_TIMEOUT_MS
}

let pending: Promise<void> | null = null

// Never throws: without the aggregate the shell falls back to the per-endpoint calls.
export function primeShellCache(qc: QueryClient): Promise<void> {
  pending ??= fetch('/api/v1/shell', {
    credentials: 'same-origin',
    signal: AbortSignal.timeout(shellSeedTimeoutMs()),
  })
    .then(async (res) => {
      if (!res.ok) return
      seedShellCache(qc, (await res.json()) as ShellOverview)
    })
    .catch(() => undefined)
  return pending
}

// The root loader awaits this so its ensureQueryData calls hit the seed, but
// never longer than the seed timeout: a slow aggregate must not hold first paint.
export function shellCacheReady(): Promise<void> {
  if (!pending) return Promise.resolve()
  let timer: ReturnType<typeof setTimeout> | undefined
  const cap = new Promise<void>((resolve) => {
    timer = setTimeout(resolve, shellSeedTimeoutMs())
  })
  return Promise.race([pending, cap]).finally(() => clearTimeout(timer))
}

export function resetShellCacheForTest(): void {
  pending = null
}
