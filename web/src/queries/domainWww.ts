// Apex and www DNS plan and apply (internal/api/dns_www_plan.go, dns_www_apply.go).
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { domainCutoverKeys } from './domainCutover'

export type WwwMode = 'both' | 'apex_only' | 'www_only'
export type WwwCanonical = 'apex' | 'www'

export interface WwwChoice {
  mode: WwwMode
  canonical: WwwCanonical
}

export interface WwwExistingRecord {
  name: string
  type: string
  value: string
  ttl_seconds: number
}

export interface WwwPlanRecord {
  name: string
  type: string
  value: string
  ttl_seconds: number
  outcome: 'created' | 'updated' | 'unchanged' | 'conflict'
  replaces: WwwExistingRecord[]
}

export interface WwwRedirect {
  from: string
  to: string
}

export interface WwwPlan {
  domain: string
  provider: string
  zone: string
  mode: WwwMode
  canonical: WwwCanonical
  records: WwwPlanRecord[]
  redirect: WwwRedirect | null
  message: string
}

export interface WwwApplyRecord extends WwwPlanRecord {
  result: string
  message?: string
}

export interface WwwApplyResult {
  domain: string
  applied: boolean
  records: WwwApplyRecord[]
  redirect: WwwRedirect | null
  redirect_applied: boolean
  redirect_message?: string
  message?: string
}

export const domainWwwKeys = {
  plan: (app: string, domain: string, choice: WwwChoice) =>
    [
      'domains',
      domain,
      'www-plan',
      app,
      choice.mode,
      choice.canonical,
    ] as const,
}

function base(app: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/domains/${encodeURIComponent(domain)}/dns`
}

export function domainWwwPlanQueryOptions(
  app: string,
  domain: string,
  choice: WwwChoice,
) {
  return queryOptions({
    queryKey: domainWwwKeys.plan(app, domain, choice),
    queryFn: async (): Promise<WwwPlan> => {
      const qs = new URLSearchParams({
        mode: choice.mode,
        canonical: choice.canonical,
      })
      const res = await fetch(`${base(app, domain)}/plan?${qs.toString()}`)
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `www plan failed: ${res.status}`),
        )
      }
      return (await res.json()) as WwwPlan
    },
    retry: false,
  })
}

export interface WwwApplyInput extends WwwChoice {
  replace: boolean
}

export function useApplyDomainWww(app: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<WwwApplyResult, ApiError, WwwApplyInput>({
    mutationFn: async (input) => {
      const res = await fetch(`${base(app, domain)}/apply`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...input, confirm: true }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `www apply failed: ${res.status}`),
        )
      }
      return (await res.json()) as WwwApplyResult
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: ['domains', domain, 'www-plan'],
      })
      void queryClient.invalidateQueries({
        queryKey: domainCutoverKeys.one(domain),
      })
    },
  })
}
