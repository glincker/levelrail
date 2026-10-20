import { queryOptions, useMutation } from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import { setDemoInfo, type DemoInfo } from '../lib/demoStore'

export const demoKeys = { info: ['demo'] as const }

// Never throws: a failed probe means "not a demo", so a real instance is unaffected.
export async function fetchDemoInfo(signal?: AbortSignal): Promise<DemoInfo> {
  try {
    const res = await fetch('/api/v1/demo', { signal })
    if (!res.ok) {
      setDemoInfo({ enabled: false })
      return { enabled: false }
    }
    const info = (await res.json()) as DemoInfo
    setDemoInfo(info)
    return info
  } catch {
    return { enabled: false }
  }
}

export function demoQueryOptions() {
  return queryOptions({
    queryKey: demoKeys.info,
    queryFn: ({ signal }) => fetchDemoInfo(signal),
    staleTime: Infinity,
    retry: false,
  })
}

interface DemoErrorBody {
  error?: string
  message?: string
}

export async function createDemoSession(humanToken: string): Promise<void> {
  const res = await fetch('/api/v1/demo/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(humanToken ? { humanToken } : {}),
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as DemoErrorBody | null
    throw new ApiError(
      res.status,
      body?.message ?? body?.error ?? `demo session failed: ${res.status}`,
    )
  }
}

export function useDemoSession() {
  return useMutation<void, ApiError, string>({ mutationFn: createDemoSession })
}

export type DemoCtaKind = 'install' | 'github' | 'docs' | 'banner_dismiss'

// Fire-and-forget aggregate click counter; failures are irrelevant to the visitor.
export function sendDemoCta(kind: DemoCtaKind): void {
  void fetch('/api/v1/demo/cta', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ kind }),
    keepalive: true,
  }).catch(() => undefined)
}
