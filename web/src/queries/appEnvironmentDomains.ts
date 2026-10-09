// GET /api/v1/apps/{name}/environment-domains and PATCH
// /api/v1/apps/{name}/domains with an `environment` field
// (internal/api/app_environment_domains.go): one app carrying a domain
// set per environment.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { domainKeys } from './domains'

export interface EnvironmentDomainSet {
  environment_id: string
  name: string
  kind: string
  active: boolean
  domains: string[]
}

// Mirrors internal/api's appEnvironmentDomainsResource.
export interface AppEnvironmentDomains {
  app: string
  active_environment_id?: string
  default_domains: string[]
  routed_domains: string[]
  environments: EnvironmentDomainSet[]
}

export const appEnvironmentDomainKeys = {
  detail: (appName: string) =>
    ['apps', appName, 'environment-domains'] as const,
}

export function appEnvironmentDomainsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appEnvironmentDomainKeys.detail(appName),
    queryFn: async (): Promise<AppEnvironmentDomains> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/environment-domains`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch environment domains failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as AppEnvironmentDomains
    },
    staleTime: 15_000,
  })
}

export function useAppEnvironmentDomains(appName: string) {
  return useQuery(appEnvironmentDomainsQueryOptions(appName))
}

export interface EditDomainsInput {
  environment?: string
  add?: string[]
  remove?: string[]
}

export interface EditDomainsResult {
  app: string
  domains: string[]
  changed: boolean
  environment_id?: string
}

async function editAppDomains(
  appName: string,
  input: EditDomainsInput,
): Promise<EditDomainsResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/domains`,
    {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `edit domains failed: ${res.status}`),
    )
  }
  return (await res.json()) as EditDomainsResult
}

export function useEditAppDomains(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: EditDomainsInput) => editAppDomains(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appEnvironmentDomainKeys.detail(appName),
      })
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: domainKeys.all })
    },
  })
}
