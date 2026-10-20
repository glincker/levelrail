// Fetchers and hooks for the supply chain routes (internal/api/supplychain.go).
// Non-suspense on purpose: a control plane without the feature (501) or a
// deploy without an SBOM (404) must never block the page the section sits on.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  SbomSummary,
  SupplyChainSettings,
  SupplyChainSettingsInput,
  VulnReport,
} from '../types/supplyChain'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { deploymentKeys } from './deployments'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const supplyChainKeys = {
  settings: (appName: string) =>
    [...appKeys.detail(appName), 'supply-chain'] as const,
  sbom: (appName: string, deploymentId: string) =>
    [...appKeys.detail(appName), 'supply-chain', 'sbom', deploymentId] as const,
  vulns: (appName: string, deploymentId: string) =>
    [
      ...appKeys.detail(appName),
      'supply-chain',
      'vulns',
      deploymentId,
    ] as const,
}

const SETTINGS_STALE_MS = 30_000

function appUrl(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/supply-chain${suffix}`
}

function deploymentUrl(
  appName: string,
  deploymentId: string,
  leaf: string,
): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/deployments/${encodeURIComponent(deploymentId)}/${leaf}`
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
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function supplyChainSettingsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: supplyChainKeys.settings(appName),
    queryFn: () =>
      request<SupplyChainSettings>(
        appUrl(appName),
        undefined,
        'fetch supply chain settings',
      ),
    retry: false,
    staleTime: SETTINGS_STALE_MS,
  })
}

export function useSupplyChainSettings(appName: string) {
  return useQuery(supplyChainSettingsQueryOptions(appName))
}

export function useSbom(
  appName: string,
  deploymentId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: supplyChainKeys.sbom(appName, deploymentId),
    queryFn: () =>
      request<SbomSummary>(
        deploymentUrl(appName, deploymentId, 'sbom'),
        undefined,
        'fetch sbom',
      ),
    enabled,
    retry: false,
  })
}

export function useVulnerabilities(
  appName: string,
  deploymentId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: supplyChainKeys.vulns(appName, deploymentId),
    queryFn: () =>
      request<VulnReport>(
        deploymentUrl(appName, deploymentId, 'vulnerabilities'),
        undefined,
        'fetch vulnerabilities',
      ),
    enabled,
    retry: false,
  })
}

export function useSetSupplyChain(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<SupplyChainSettings, ApiError, SupplyChainSettingsInput>({
    mutationFn: (input) =>
      request<SupplyChainSettings>(
        appUrl(appName),
        jsonInit('PUT', input),
        'save supply chain settings',
      ),
    onSuccess: (settings) => {
      queryClient.setQueryData(supplyChainKeys.settings(appName), settings)
    },
  })
}

export function useOverrideScanGate(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<SupplyChainSettings, ApiError, string>({
    mutationFn: (reason) =>
      request<SupplyChainSettings>(
        appUrl(appName, '/override'),
        jsonInit('POST', { reason }),
        'arm scan gate override',
      ),
    onSuccess: (settings) => {
      queryClient.setQueryData(supplyChainKeys.settings(appName), settings)
    },
  })
}

export function useScanDeployment(appName: string, deploymentId: string) {
  const queryClient = useQueryClient()
  return useMutation<VulnReport, ApiError, void>({
    mutationFn: () =>
      request<VulnReport>(
        deploymentUrl(appName, deploymentId, 'scan'),
        jsonInit('POST'),
        'scan deployment',
      ),
    onSuccess: (report) => {
      queryClient.setQueryData(
        supplyChainKeys.vulns(appName, deploymentId),
        report,
      )
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
      void queryClient.invalidateQueries({ queryKey: deploymentKeys.all })
    },
  })
}
