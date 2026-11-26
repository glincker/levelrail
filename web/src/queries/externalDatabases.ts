import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  ExternalCandidate,
  ExternalDatabase,
  ExternalDatabaseRequest,
  ExternalParsedConnection,
  ExternalProbeResult,
  ExternalProvider,
} from '../types/externalDatabase'
import { databaseKeys } from './databases'

const BASE = '/api/v1/external-databases'

async function send<T>(
  method: string,
  path: string,
  body: unknown,
  failure: string,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export const externalDatabaseKeys = {
  candidates: (nodeId: string) =>
    [...databaseKeys.all, 'external-candidates', nodeId] as const,
  providers: () => [...databaseKeys.all, 'external-providers'] as const,
}

export function useExternalCandidates(nodeId: string, enabled: boolean) {
  return useQuery({
    queryKey: externalDatabaseKeys.candidates(nodeId),
    queryFn: () =>
      send<ExternalCandidate[]>(
        'GET',
        `${BASE}/candidates?node_id=${encodeURIComponent(nodeId)}`,
        undefined,
        'list adoptable containers failed',
      ),
    enabled,
    retry: false,
  })
}

export function useTestExternalDatabase() {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (req: ExternalDatabaseRequest) =>
      send<ExternalProbeResult>(
        'POST',
        `${BASE}/test`,
        req,
        'test connection failed',
      ),
  })
}

function useInvalidatingMutation<V, R>(fn: (v: V) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: databaseKeys.all })
    },
  })
}

export function useConnectExternalDatabase() {
  return useInvalidatingMutation((req: ExternalDatabaseRequest) =>
    send<ExternalDatabase>('POST', BASE, req, 'connect database failed'),
  )
}

export function useAdoptExternalDatabase() {
  return useInvalidatingMutation((req: ExternalDatabaseRequest) =>
    send<ExternalDatabase>(
      'POST',
      `${BASE}/adopt`,
      req,
      'adopt database failed',
    ),
  )
}

export function useProbeExternalDatabase() {
  return useInvalidatingMutation((name: string) =>
    send<ExternalProbeResult>(
      'POST',
      `${BASE}/${encodeURIComponent(name)}/probe`,
      undefined,
      'probe failed',
    ),
  )
}

export function useDeleteExternalDatabase() {
  return useInvalidatingMutation((v: { name: string; force: boolean }) =>
    send<void>(
      'DELETE',
      `${BASE}/${encodeURIComponent(v.name)}${v.force ? '?force=true' : ''}`,
      undefined,
      'delete failed',
    ),
  )
}

export function useRevealExternalPassword() {
  return useMutation({
    mutationFn: (name: string) =>
      send<{ password: string }>(
        'GET',
        `${BASE}/${encodeURIComponent(name)}/password`,
        undefined,
        'reveal failed',
      ),
  })
}

export function useExternalProviders(enabled: boolean) {
  return useQuery({
    queryKey: externalDatabaseKeys.providers(),
    queryFn: () =>
      send<ExternalProvider[]>(
        'GET',
        `${BASE}/providers`,
        undefined,
        'list hosted providers failed',
      ),
    enabled,
    staleTime: Infinity,
    retry: false,
  })
}

export function useParseExternalUrl() {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (url: string) =>
      send<ExternalParsedConnection>(
        'POST',
        `${BASE}/parse`,
        { url },
        'parse connection string failed',
      ),
  })
}

// A hosted save refused with 422: the probe failed and nothing was stored.
export class HostedProbeBlockedError extends ApiError {
  readonly probe: ExternalProbeResult | null

  constructor(message: string, probe: ExternalProbeResult | null) {
    super(422, message)
    this.probe = probe
  }
}

export function useConnectHostedDatabase() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (req: ExternalDatabaseRequest) => {
      const res = await fetch(BASE, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (res.status === 422) {
        const body = (await res.json().catch(() => null)) as {
          error?: string
          probe?: ExternalProbeResult
        } | null
        throw new HostedProbeBlockedError(
          body?.error ?? 'connection test failed',
          body?.probe ?? null,
        )
      }
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `connect database failed: ${res.status}`),
        )
      }
      return (await res.json()) as ExternalDatabase
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: databaseKeys.all })
    },
  })
}
