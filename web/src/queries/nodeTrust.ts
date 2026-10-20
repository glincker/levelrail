// Trust summary, security events and renew-now for a node
// (internal/api/node_trust.go). All routes are root-only.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { NodeSecurityEvent, NodeTrustResource } from '../types/nodeTrust'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

const TRUST_REFRESH_MS = 15_000
export const SECURITY_EVENTS_LIMIT = 50

export const nodeTrustKeys = {
  summary: (id: string) => [...nodeKeys.detail(id), 'security'] as const,
  events: (id: string, rejectedOnly: boolean) =>
    [...nodeKeys.detail(id), 'security-events', rejectedOnly] as const,
}

async function getJSON<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function nodeTrustQueryOptions(id: string) {
  return queryOptions({
    queryKey: nodeTrustKeys.summary(id),
    queryFn: () =>
      getJSON<NodeTrustResource>(
        `/api/v1/nodes/${encodeURIComponent(id)}/security`,
        'load node security',
      ),
    refetchInterval: TRUST_REFRESH_MS,
  })
}

export function useNodeTrust(id: string, enabled: boolean) {
  return useQuery({ ...nodeTrustQueryOptions(id), enabled })
}

export function useNodeSecurityEvents(
  id: string,
  rejectedOnly: boolean,
  enabled: boolean,
) {
  return useQuery({
    queryKey: nodeTrustKeys.events(id, rejectedOnly),
    queryFn: () => {
      const q = new URLSearchParams({ limit: String(SECURITY_EVENTS_LIMIT) })
      if (rejectedOnly) q.set('outcome', 'rejected')
      return getJSON<NodeSecurityEvent[]>(
        `/api/v1/nodes/${encodeURIComponent(id)}/security/events?${q.toString()}`,
        'load security events',
      )
    },
    refetchInterval: TRUST_REFRESH_MS,
    enabled,
  })
}

export async function renewNodeCert(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(id)}/cert/renew`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `renew certificate failed: ${res.status}`),
    )
  }
}

export type CertDrillStepId =
  'preflight' | 'request' | 'new_certificate' | 'still_online'

export interface CertDrillStep {
  id: CertDrillStepId
  status: 'pass' | 'fail' | 'skipped'
  detail: string
}

export interface CertDrillResult {
  ok: boolean
  steps: CertDrillStep[]
  before: { serial?: string; generation: number }
  after: { serial?: string; generation: number }
}

export function useNodeCertDrill() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<CertDrillResult> => {
      const res = await fetch(
        `/api/v1/nodes/${encodeURIComponent(id)}/cert/drill`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `certificate drill failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as CertDrillResult
    },
    onSettled: (_data, _err, id) => {
      void queryClient.invalidateQueries({ queryKey: nodeKeys.detail(id) })
    },
  })
}

export function useRenewNodeCert() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: renewNodeCert,
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: nodeKeys.detail(id) })
    },
  })
}
