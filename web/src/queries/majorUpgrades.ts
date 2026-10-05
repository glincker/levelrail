// Fetchers and hooks for guarded Postgres major upgrades
// (internal/api/database_major_upgrade.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface MajorUpgrade {
  id: string
  database_name: string
  from_version: string
  to_version: string
  status: 'running' | 'succeeded' | 'failed' | 'rolled_back'
  phase?: string
  snapshot_volume?: string
  error?: string
  started_at: string
  finished_at?: string
}

const POLL_MS = 3_000

export const majorUpgradeKeys = {
  list: (name: string) => ['databases', name, 'major-upgrades'] as const,
}

function base(name: string): string {
  return `/api/v1/databases/${encodeURIComponent(name)}`
}

async function send<T>(
  url: string,
  method: string,
  body: unknown,
  context: string,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${context} failed: ${res.status}`),
    )
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T)
}

export function useMajorUpgrades(name: string) {
  return useQuery({
    queryKey: majorUpgradeKeys.list(name),
    queryFn: () =>
      send<MajorUpgrade[]>(
        `${base(name)}/major-upgrades`,
        'GET',
        undefined,
        'list major upgrades',
      ),
    refetchInterval: (query) =>
      query.state.data?.some((u) => u.status === 'running') ? POLL_MS : false,
  })
}

function useUpgradeMutation<V>(
  name: string,
  fn: (vars: V) => Promise<unknown>,
) {
  const queryClient = useQueryClient()
  return useMutation<unknown, ApiError, V>({
    mutationFn: fn,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: majorUpgradeKeys.list(name) }),
  })
}

export function useStartMajorUpgrade(name: string) {
  return useUpgradeMutation<string>(name, (version) =>
    send(
      `${base(name)}/major-upgrade`,
      'POST',
      { version, confirm: name },
      'start major upgrade',
    ),
  )
}

export function useRollbackMajorUpgrade(name: string) {
  return useUpgradeMutation<string>(name, (id) =>
    send(
      `${base(name)}/major-upgrades/${encodeURIComponent(id)}/rollback`,
      'POST',
      { confirm: name },
      'roll back major upgrade',
    ),
  )
}

export function useDiscardMajorUpgradeSnapshot(name: string) {
  return useUpgradeMutation<string>(name, (id) =>
    send(
      `${base(name)}/major-upgrades/${encodeURIComponent(id)}/snapshot`,
      'DELETE',
      undefined,
      'discard snapshot',
    ),
  )
}
