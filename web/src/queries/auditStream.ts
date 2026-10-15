// Query keys, fetchers and mutations for /api/v1/audit-sinks and
// /api/v1/audit-log/verify (internal/api/audit_sinks.go, audit_chain.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  AuditChainReport,
  AuditSink,
  AuditSinkDelivery,
  AuditSinkTestResult,
  CreateAuditSinkRequest,
  UpdateAuditSinkRequest,
} from '../types/auditStream'

export const auditStreamKeys = {
  all: ['audit-stream'] as const,
  chain: () => [...auditStreamKeys.all, 'chain'] as const,
  sinks: () => [...auditStreamKeys.all, 'sinks'] as const,
  deliveries: (id: string) =>
    [...auditStreamKeys.all, 'deliveries', id] as const,
}

// Cursor and failure counters move every few seconds while streaming.
const REFETCH_MS = 10_000

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

const base = '/api/v1/audit-sinks'

export function useAuditChain() {
  return useQuery(
    queryOptions({
      queryKey: auditStreamKeys.chain(),
      queryFn: () =>
        request<AuditChainReport>(
          '/api/v1/audit-log/verify',
          undefined,
          'verify audit chain',
        ),
      retry: false,
    }),
  )
}

export function useAuditSinks() {
  return useQuery(
    queryOptions({
      queryKey: auditStreamKeys.sinks(),
      queryFn: () => request<AuditSink[]>(base, undefined, 'fetch audit sinks'),
      refetchInterval: REFETCH_MS,
      retry: false,
    }),
  )
}

export function useAuditSinkDeliveries(id: string, enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: auditStreamKeys.deliveries(id),
      queryFn: () =>
        request<AuditSinkDelivery[]>(
          `${base}/${encodeURIComponent(id)}/deliveries`,
          undefined,
          'fetch deliveries',
        ),
      refetchInterval: REFETCH_MS,
      enabled,
    }),
  )
}

function useInvalidatingMutation<TVars, TData>(
  fn: (vars: TVars) => Promise<TData>,
) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: auditStreamKeys.all }),
  })
}

export function useCreateAuditSink() {
  return useInvalidatingMutation((req: CreateAuditSinkRequest) =>
    request<AuditSink>(base, jsonInit('POST', req), 'create audit sink'),
  )
}

export function useUpdateAuditSink() {
  return useInvalidatingMutation(
    (vars: { id: string; body: UpdateAuditSinkRequest }) =>
      request<AuditSink>(
        `${base}/${encodeURIComponent(vars.id)}`,
        jsonInit('PATCH', vars.body),
        'update audit sink',
      ),
  )
}

export function useDeleteAuditSink() {
  return useInvalidatingMutation((id: string) =>
    request<void>(
      `${base}/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
      'delete audit sink',
    ),
  )
}

export function useTestAuditSink() {
  return useInvalidatingMutation((id: string) =>
    request<AuditSinkTestResult>(
      `${base}/${encodeURIComponent(id)}/test`,
      { method: 'POST' },
      'test audit sink',
    ),
  )
}

export function useRotateAuditSinkSecret() {
  return useInvalidatingMutation((id: string) =>
    request<AuditSink>(
      `${base}/${encodeURIComponent(id)}/rotate-secret`,
      { method: 'POST' },
      'rotate signing secret',
    ),
  )
}
