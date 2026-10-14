// Fetchers and hooks for the secret scan routes (internal/api/secretscan.go).
// Non-suspense: a control plane without the feature (501) or a deploy with no
// recorded scan (404) must never block the page the panel sits on.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  SecretScanAllowEntry,
  SecretScanPolicy,
  SecretScanRun,
  SecretScanSettings,
} from '../types/secretScan'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const secretScanKeys = {
  settings: (appName: string) =>
    [...appKeys.detail(appName), 'secret-scan'] as const,
  run: (appName: string, deploymentId: string) =>
    [...appKeys.detail(appName), 'secret-scan', 'run', deploymentId] as const,
}

const SETTINGS_STALE_MS = 30_000

function appUrl(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/secret-scan${suffix}`
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function secretScanSettingsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: secretScanKeys.settings(appName),
    queryFn: () =>
      request<SecretScanSettings>(
        appUrl(appName),
        undefined,
        'fetch secret scan settings',
      ),
    retry: false,
    staleTime: SETTINGS_STALE_MS,
  })
}

export function useSecretScanSettings(appName: string) {
  return useQuery(secretScanSettingsQueryOptions(appName))
}

export function useDeploymentSecretScan(appName: string, deploymentId: string) {
  return useQuery({
    queryKey: secretScanKeys.run(appName, deploymentId),
    queryFn: () =>
      request<SecretScanRun>(
        `/api/v1/apps/${encodeURIComponent(appName)}/deployments/${encodeURIComponent(deploymentId)}/secret-scan`,
        undefined,
        'fetch secret scan',
      ),
    retry: false,
  })
}

export function useSetSecretScanPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<SecretScanSettings, ApiError, SecretScanPolicy>({
    mutationFn: (policy) =>
      request<SecretScanSettings>(
        appUrl(appName),
        jsonInit('PUT', { policy }),
        'save secret scan policy',
      ),
    onSuccess: (settings) => {
      queryClient.setQueryData(secretScanKeys.settings(appName), settings)
    },
  })
}

export function useAllowSecretFingerprint(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    SecretScanAllowEntry,
    ApiError,
    { fingerprint: string; reason: string }
  >({
    mutationFn: (input) =>
      request<SecretScanAllowEntry>(
        appUrl(appName, '/allow'),
        jsonInit('POST', input),
        'allow-list fingerprint',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: secretScanKeys.settings(appName),
      })
    },
  })
}

export function useUnallowSecretFingerprint(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (fingerprint) =>
      request<void>(
        appUrl(appName, `/allow/${encodeURIComponent(fingerprint)}`),
        { method: 'DELETE' },
        'remove allow-list entry',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: secretScanKeys.settings(appName),
      })
    },
  })
}
