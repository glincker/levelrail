import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  DatabaseUpgrades,
  UpgradeNowInput,
  UpgradePolicy,
  UpgradePolicyInput,
  UpgradeRun,
} from '../types/databaseUpgrades'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

const POLL_MS = 5_000

export const databaseUpgradeKeys = {
  detail: (name: string) => [...databaseKeys.detail(name), 'upgrades'] as const,
  platform: ['settings', 'database-upgrades'] as const,
}

const base = (name: string) => `/api/v1/databases/${encodeURIComponent(name)}`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  label: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${label} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function databaseUpgradesQueryOptions(name: string) {
  return queryOptions({
    queryKey: databaseUpgradeKeys.detail(name),
    queryFn: () =>
      request<DatabaseUpgrades>(
        `${base(name)}/upgrades`,
        undefined,
        'fetch upgrades',
      ),
    staleTime: 10_000,
    retry: false,
    refetchInterval: (query) => (query.state.data?.active ? POLL_MS : false),
  })
}

export function useDatabaseUpgrades(name: string) {
  return useQuery(databaseUpgradesQueryOptions(name))
}

export function useSetUpgradePolicy(name: string) {
  const qc = useQueryClient()
  return useMutation<UpgradePolicy, ApiError, UpgradePolicyInput>({
    mutationFn: (input) =>
      request(
        `${base(name)}/upgrade-policy`,
        json('PUT', input),
        'save upgrade policy',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: databaseUpgradeKeys.detail(name) }),
  })
}

export function useUpgradeNow(name: string) {
  const qc = useQueryClient()
  return useMutation<UpgradeRun, ApiError, UpgradeNowInput>({
    mutationFn: (input) =>
      request(`${base(name)}/upgrade-now`, json('POST', input), 'upgrade'),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: databaseUpgradeKeys.detail(name) }),
  })
}

export function usePlatformUpgradePolicy() {
  return useQuery({
    queryKey: databaseUpgradeKeys.platform,
    queryFn: () =>
      request<UpgradePolicy>(
        '/api/v1/settings/database-upgrades',
        undefined,
        'fetch platform upgrade policy',
      ),
    retry: false,
  })
}

export function useSetPlatformUpgradePolicy() {
  const qc = useQueryClient()
  return useMutation<UpgradePolicy, ApiError, UpgradePolicyInput>({
    mutationFn: (input) =>
      request(
        '/api/v1/settings/database-upgrades',
        json('PUT', input),
        'save platform upgrade policy',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: databaseUpgradeKeys.platform }),
  })
}
