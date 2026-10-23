// /api/v1/apps/{name}/split (internal/api/split_testing_http.go): send a share
// of an app's visitors to another deploy, sticky per visitor.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type SplitVariantKind = 'production' | 'branch' | 'release'

export interface SplitVariant {
  id: string
  label: string
  kind: SplitVariantKind
  ref?: string
  weight: number
  status: 'live' | 'ready' | 'unavailable'
  host?: string
  requests: number
  share: number
  qa_link?: string
}

export interface SplitBucket {
  bucket: string
  counts: Record<string, number>
}

export interface SplitTest {
  active: boolean
  supported: boolean
  unsupported_reason?: string
  test_id?: string
  status?: 'running' | 'stopped'
  cookie_name?: string
  cookie_ttl_secs?: number
  override_param?: string
  created_at?: string
  ended_at?: string
  end_reason?: string
  total_requests: number
  variants: SplitVariant[]
  hourly?: SplitBucket[]
}

export interface SplitVariantInput {
  label?: string
  kind: Exclude<SplitVariantKind, 'production'>
  ref: string
  weight: number
}

export interface SplitStartBody {
  variants: SplitVariantInput[]
  cookie_name?: string
  cookie_ttl?: string
}

const LIVE_REFRESH_MS = 10_000

export const splitKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'split'] as const,
}

async function request(
  appName: string,
  method: string,
  suffix: string,
  body?: unknown,
): Promise<SplitTest> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/split${suffix}`,
    {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `split test request failed: ${res.status}`),
    )
  }
  return (await res.json()) as SplitTest
}

export function splitQueryOptions(appName: string) {
  return queryOptions({
    queryKey: splitKeys.detail(appName),
    queryFn: () => request(appName, 'GET', ''),
    refetchInterval: (query) =>
      query.state.data?.active ? LIVE_REFRESH_MS : false,
  })
}

export function useSplitTest(appName: string) {
  return useQuery(splitQueryOptions(appName))
}

export function useSplitActions(appName: string) {
  const queryClient = useQueryClient()
  const onSuccess = (result: SplitTest) => {
    queryClient.setQueryData(splitKeys.detail(appName), result)
  }
  const start = useMutation<SplitTest, ApiError, SplitStartBody>({
    mutationFn: (body) => request(appName, 'POST', '', body),
    onSuccess,
  })
  const setWeights = useMutation<SplitTest, ApiError, Record<string, number>>({
    mutationFn: (weights) => request(appName, 'PUT', '', { weights }),
    onSuccess,
  })
  const promote = useMutation<SplitTest, ApiError, string>({
    mutationFn: (variant) => request(appName, 'POST', '/promote', { variant }),
    onSuccess: (result) => {
      onSuccess(result)
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(appName),
        exact: true,
      })
    },
  })
  const stop = useMutation<SplitTest, ApiError, void>({
    mutationFn: () => request(appName, 'DELETE', ''),
    onSuccess,
  })
  return { start, setWeights, promote, stop }
}
