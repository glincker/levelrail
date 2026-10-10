// Fetchers for the IAM authoring endpoints under /api/v1/iam (catalog,
// resource picker, simulator, analyzer, preview, versions). Read-only
// computations: nothing here persists a policy.

import { queryOptions, useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { PolicyDocument, PrincipalType } from './iamPolicies'

export type AbilityRisk = 'read' | 'write' | 'sensitive' | 'root'

export interface AbilityInfo {
  id: string
  group: string
  risk: AbilityRisk
  title: string
  description: string
}

export interface IamCatalog {
  abilities: AbilityInfo[]
  conditions: { version: number; supported: string[]; planned: string[] }
}

export interface IamProjectRef {
  id: string
  name: string
  environment_ids: string[]
}

export interface IamEnvironmentRef {
  id: string
  name: string
  kind: string
  project_id: string
  apps: number
  databases: number
}

export interface IamResourceRef {
  name: string
  environment_id?: string
  environment_kind?: string
}

export interface IamResources {
  projects: IamProjectRef[]
  environments: IamEnvironmentRef[]
  apps: IamResourceRef[]
  databases: IamResourceRef[]
}

export interface ResourceMatch {
  pattern: string
  apps: number
  databases: number
  sample: string[]
  error?: string
}

export interface IamPrincipal {
  principal_type: PrincipalType
  principal_id: string
  name: string
  detail?: string
  abilities: string[]
  active: boolean
  last_used_at?: string
  policy_ids: string[]
}

export interface StatementRef {
  policy_id: string
  policy_name: string
  statement_index: number
  effect: 'Allow' | 'Deny'
  action: string[]
  resource: string[]
}

export type DecidedBy =
  | 'explicit_deny'
  | 'base_ability'
  | 'explicit_allow'
  | 'no_grant'
  | 'hidden'
  | 'inactive_principal'

export interface Simulation {
  principal_type: PrincipalType
  principal_id: string
  principal_name: string
  action: string
  resource: string
  allowed: boolean
  decided_by: DecidedBy
  deciding_statement: StatementRef | null
  matched_statements: StatementRef[]
  base_ability_grants: boolean
  environment_resources: string[]
}

export interface EffectiveAbility {
  ability: string
  risk: AbilityRisk
  flat: boolean
  all: boolean
  allowed: number
  total: number
  apps: string[]
  databases: string[]
  granted_by_policy: number
  denied_by_policy: number
}

export interface IamEffective {
  principal: IamPrincipal
  policies: { id: string; name: string }[]
  restricted: boolean
  abilities: EffectiveAbility[]
}

export type FindingSeverity = 'critical' | 'high' | 'medium' | 'low' | 'info'

export interface IamFinding {
  kind: string
  severity: FindingSeverity
  policy_id?: string
  policy_name?: string
  statement_index?: number
  principal_type?: PrincipalType
  principal_id?: string
  message: string
  fix: string
}

export interface IamAnalysis {
  score: number
  counts: Partial<Record<FindingSeverity, number>>
  findings: IamFinding[]
}

export interface FieldIssue {
  path: string
  message: string
  severity: 'error' | 'warning'
}

export interface ValidateResult {
  valid: boolean
  issues: FieldIssue[]
  findings: IamFinding[]
}

export interface ResourceDelta {
  ability: string
  risk: AbilityRisk
  count: number
  apps: string[]
  databases: string[]
}

export interface PrincipalChange {
  principal: IamPrincipal
  gains: ResourceDelta[]
  losses: ResourceDelta[]
  recently_used: boolean
}

export interface PreviewResult {
  valid: boolean
  issues: FieldIssue[]
  changes: PrincipalChange[]
  guard: { blocked: boolean; reason?: string }
  usage_note: string
}

export interface PreviewRequest {
  policy_id?: string
  document?: PolicyDocument
  attach?: { principal_type: PrincipalType; principal_id: string }[]
  detach?: { principal_type: PrincipalType; principal_id: string }[]
}

export interface StatementChange {
  change: 'added' | 'removed'
  statement: PolicyDocument['Statement'][number]
}

export interface PolicyVersion {
  version: number
  name: string
  description: string
  document: PolicyDocument
  actor: string
  created_at: string
  changes: StatementChange[]
}

export interface RenderedTemplate {
  name: string
  description: string
  document: PolicyDocument
}

export const iamBuilderKeys = {
  all: ['iam-builder'] as const,
  catalog: () => [...iamBuilderKeys.all, 'catalog'] as const,
  resources: () => [...iamBuilderKeys.all, 'resources'] as const,
  principals: () => [...iamBuilderKeys.all, 'principals'] as const,
  effective: (type: string, id: string) =>
    [...iamBuilderKeys.all, 'effective', type, id] as const,
  analysis: () => [...iamBuilderKeys.all, 'analysis'] as const,
  versions: (policyId: string) =>
    [...iamBuilderKeys.all, 'versions', policyId] as const,
}

async function request<T>(
  path: string,
  init?: { method: 'POST'; body: unknown },
): Promise<T> {
  const res = await fetch(
    path,
    init
      ? {
          method: init.method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(init.body),
        }
      : undefined,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export const iamCatalogQueryOptions = () =>
  queryOptions({
    queryKey: iamBuilderKeys.catalog(),
    queryFn: () => request<IamCatalog>('/api/v1/iam/catalog'),
    staleTime: 5 * 60_000,
  })

export const iamResourcesQueryOptions = () =>
  queryOptions({
    queryKey: iamBuilderKeys.resources(),
    queryFn: () => request<IamResources>('/api/v1/iam/resources'),
  })

export const iamPrincipalsQueryOptions = () =>
  queryOptions({
    queryKey: iamBuilderKeys.principals(),
    queryFn: () => request<IamPrincipal[]>('/api/v1/iam/principals'),
  })

export const iamEffectiveQueryOptions = (type: string, id: string) =>
  queryOptions({
    queryKey: iamBuilderKeys.effective(type, id),
    queryFn: () =>
      request<IamEffective>(
        `/api/v1/iam/principals/${encodeURIComponent(type)}/${encodeURIComponent(id)}/effective`,
      ),
    enabled: Boolean(type && id),
  })

export const iamAnalysisQueryOptions = () =>
  queryOptions({
    queryKey: iamBuilderKeys.analysis(),
    queryFn: () => request<IamAnalysis>('/api/v1/iam/analyze'),
  })

export const policyVersionsQueryOptions = (policyId: string) =>
  queryOptions({
    queryKey: iamBuilderKeys.versions(policyId),
    queryFn: () =>
      request<PolicyVersion[]>(
        `/api/v1/iam/policies/${encodeURIComponent(policyId)}/versions`,
      ),
  })

export function matchResources(resources: string[]): Promise<ResourceMatch[]> {
  return request<ResourceMatch[]>('/api/v1/iam/resources/match', {
    method: 'POST',
    body: { resources },
  })
}

export function validatePolicy(
  document: PolicyDocument,
): Promise<ValidateResult> {
  return request<ValidateResult>('/api/v1/iam/policies/validate', {
    method: 'POST',
    body: { document },
  })
}

export function previewChange(req: PreviewRequest): Promise<PreviewResult> {
  return request<PreviewResult>('/api/v1/iam/preview', {
    method: 'POST',
    body: req,
  })
}

export function renderTemplate(
  id: string,
  params: Record<string, string>,
): Promise<RenderedTemplate> {
  return request<RenderedTemplate>(
    `/api/v1/iam/policy-templates/${encodeURIComponent(id)}/render`,
    { method: 'POST', body: { params } },
  )
}

export interface SimulateInput {
  principalType: PrincipalType
  principalId: string
  action: string
  resource: string
}

export function useSimulate() {
  return useMutation<Simulation, ApiError, SimulateInput>({
    mutationFn: (i) => {
      const q = new URLSearchParams({
        principal_type: i.principalType,
        principal_id: i.principalId,
        action: i.action,
        resource: i.resource,
      })
      return request<Simulation>(`/api/v1/iam/simulate?${q.toString()}`)
    },
  })
}
