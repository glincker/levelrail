import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeDomains, NodeOnboarding } from '../types/nodeOnboarding'

const POLL_MS = 3_000
// A failing node keeps being re-read so a fix applied on it shows up.
const RECHECK_MS = 15_000

export const nodeOnboardingKeys = {
  byName: (name: string) => ['nodes', 'onboarding', 'by-name', name] as const,
  one: (id: string) => ['nodes', id, 'onboarding'] as const,
  domains: () => ['nodes', 'domains'] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url, { cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

// The node row only exists once its agent has enrolled, so the wizard finds
// it by the name it asked the agent to use.
export function useNodeByName(name: string, enabled: boolean) {
  return useQuery({
    queryKey: nodeOnboardingKeys.byName(name),
    queryFn: async () => {
      const rows = await getJson<NodeResource[]>(
        `/api/v1/nodes?q=${encodeURIComponent(name)}`,
        'find node',
      )
      return Array.isArray(rows)
        ? (rows.find((n) => n.name === name) ?? null)
        : null
    },
    enabled,
    retry: false,
    refetchInterval: (q) => (q.state.data ? false : POLL_MS),
  })
}

// Polls while the node is still joining or has not reported yet.
export function useNodeOnboarding(id: string | undefined) {
  return useQuery({
    queryKey: nodeOnboardingKeys.one(id ?? ''),
    queryFn: () =>
      getJson<NodeOnboarding>(
        `/api/v1/nodes/${encodeURIComponent(id ?? '')}/onboarding`,
        'fetch node onboarding',
      ),
    enabled: Boolean(id),
    retry: false,
    refetchInterval: (q) => {
      const d = q.state.data
      if (!d || !d.reported || d.stale) return POLL_MS
      return d.summary.ready ? false : RECHECK_MS
    },
  })
}

export function useNodeDomains() {
  return useQuery({
    queryKey: nodeOnboardingKeys.domains(),
    queryFn: () =>
      getJson<NodeDomains>('/api/v1/nodes/domains', 'fetch node domains'),
    retry: false,
  })
}
