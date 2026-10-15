// Fetchers for /api/v1/deploy-policies (internal/api/deploy_policy.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type PolicyScope = 'org' | 'project' | 'environment'
export type PolicyMode = 'off' | 'warn' | 'enforce'

export const POLICY_RULE_IDS = [
  'allowed_registries',
  'require_readiness_probe',
  'max_memory_mb',
  'max_cpu',
  'forbid_privileged',
  'forbid_host_mounts',
  'required_labels',
  'max_replicas',
] as const

export type PolicyRuleId = (typeof POLICY_RULE_IDS)[number]

export interface PolicyRule {
  mode: PolicyMode
  values?: string[]
  limit?: number
}

export type PolicyRules = Partial<Record<PolicyRuleId, PolicyRule>>

export interface DeployPolicy {
  scope: PolicyScope
  scope_id: string
  rules: PolicyRules
  updated_by?: string
  updated_at?: string
}

export interface PolicyViolation {
  rule: PolicyRuleId
  code: string
  mode: PolicyMode
  message: string
  next_action: string
}

export interface PolicyCheckApp {
  app: string
  project_id?: string
  environment_id?: string
  violations: PolicyViolation[]
  would_fail: boolean
}

export interface PolicyCheckResult {
  checked: number
  failing: number
  warning: number
  apps: PolicyCheckApp[]
}

export interface PolicyCheckRequest {
  app?: string
  scope?: PolicyScope
  scope_id?: string
  rules?: PolicyRules
}

export const deployPolicyKeys = {
  all: ['deploy-policies'] as const,
  list: () => [...deployPolicyKeys.all, 'list'] as const,
}

const BASE = '/api/v1/deploy-policies'

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
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
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

const scopePath = (scope: PolicyScope, id: string) =>
  `${BASE}/${scope}/${encodeURIComponent(id)}`

export function deployPoliciesQueryOptions() {
  return queryOptions({
    queryKey: deployPolicyKeys.list(),
    queryFn: () => request<DeployPolicy[]>('GET', BASE),
    retry: false,
  })
}

export function useSaveDeployPolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { scope: PolicyScope; id: string; rules: PolicyRules }) =>
      request<DeployPolicy>('PUT', scopePath(v.scope, v.id), {
        rules: v.rules,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: deployPolicyKeys.all }),
  })
}

export function useClearDeployPolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { scope: PolicyScope; id: string }) =>
      request<DeployPolicy>('DELETE', scopePath(v.scope, v.id)),
    onSuccess: () => qc.invalidateQueries({ queryKey: deployPolicyKeys.all }),
  })
}

export function useCheckDeployPolicy() {
  return useMutation({
    mutationFn: (req: PolicyCheckRequest) =>
      request<PolicyCheckResult>('POST', `${BASE}/check`, req),
  })
}
