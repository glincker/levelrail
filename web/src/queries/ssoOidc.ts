// Fetchers and hooks for the OAuth and OIDC sign-in policy (internal/api/sso_oauth_*.go):
// group claim, role sync, JIT, allowed domains and the test login (root only),
// plus the public domain routing the sign-in page uses.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { ApiError } from '../lib/apiError'
import { ssoKeys, ssoRequest } from './sso'

export interface OAuthSSOPolicy {
  provider: string
  name: string
  enabled: boolean
  groups_supported: boolean
  groups_claim: string
  sync_roles: boolean
  default_role_id: string
  jit_provisioning: boolean
  allowed_domains: string[]
}

export type OAuthSSOPolicyInput = Omit<
  OAuthSSOPolicy,
  'provider' | 'name' | 'enabled' | 'groups_supported'
>

export interface SSOFix {
  kind: 'auto' | 'manual'
  action?: 'set_groups_claim' | 'add_domain' | 'enable_jit'
  value?: string
}

export type SSOIssueCode =
  | 'no_groups_claim'
  | 'wrong_claim_name'
  | 'email_unverified'
  | 'domain_not_allowed'
  | 'not_provisioned'
  | 'role_grants_root'
  | 'no_email'

export type SSOAccountState =
  'linked' | 'existing_by_email' | 'new_account' | 'not_provisioned'

export interface SSOIssue {
  code: SSOIssueCode
  fix: SSOFix
}

export interface SSOEvaluation {
  provider: string
  email: string
  email_verified: boolean
  groups_claim: string
  groups_found: boolean
  groups: string[]
  group_like_claims: string[]
  role_id: string
  role_source: 'mapping' | 'default'
  account_state: SSOAccountState
  sync_roles: boolean
  allowed: boolean
  refusal_code?: string
  issues: SSOIssue[]
}

export interface SSOTestResult {
  status: 'none' | 'ok' | 'failed'
  failure_code?: string
  evaluation?: SSOEvaluation
}

export interface SSORoute {
  kind: 'saml' | 'oauth'
  id: string
  name: string
  start_url: string
}

const OIDC = '/api/v1/sso/oidc'

export const ssoOidcKeys = {
  policies: () => [...ssoKeys.all, 'oidc'] as const,
}

export function oauthSSOPoliciesQueryOptions() {
  return queryOptions({
    queryKey: ssoOidcKeys.policies(),
    queryFn: () => ssoRequest<OAuthSSOPolicy[]>(OIDC),
  })
}

export function policyInput(p: OAuthSSOPolicy): OAuthSSOPolicyInput {
  return {
    groups_claim: p.groups_claim,
    sync_roles: p.sync_roles,
    default_role_id: p.default_role_id,
    jit_provisioning: p.jit_provisioning,
    allowed_domains: p.allowed_domains,
  }
}

export function useSaveOAuthSSOPolicy() {
  const queryClient = useQueryClient()
  return useMutation<
    OAuthSSOPolicy,
    ApiError,
    { provider: string; input: OAuthSSOPolicyInput }
  >({
    mutationFn: ({ provider, input }) =>
      ssoRequest<OAuthSSOPolicy>(
        `${OIDC}/${encodeURIComponent(provider)}`,
        'PUT',
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ssoKeys.all })
    },
  })
}

export function ssoTestStartURL(provider: string): string {
  return `${OIDC}/${encodeURIComponent(provider)}/test/start`
}

// One-shot: the server forgets a result once it has been read.
export function fetchSSOTestResult(provider: string): Promise<SSOTestResult> {
  return ssoRequest<SSOTestResult>(
    `${OIDC}/${encodeURIComponent(provider)}/test/result`,
  )
}

const DOMAIN =
  /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/

// Only the part after the @ ever leaves the browser.
export function emailDomain(input: string): string {
  const at = input.lastIndexOf('@')
  if (at < 0) return ''
  const domain = input
    .slice(at + 1)
    .trim()
    .toLowerCase()
  return DOMAIN.test(domain) ? domain : ''
}

// Public and unauthenticated; the answer for an unknown domain is null, the
// same as for no SSO at all.
export async function fetchSSORoute(domain: string): Promise<SSORoute | null> {
  const body = await ssoRequest<{ sso_route: SSORoute | null }>(
    `/api/v1/auth/login-options?sso_domain=${encodeURIComponent(domain)}`,
  )
  return body.sso_route
}

export function splitDomains(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((d) => d.trim().toLowerCase())
    .filter((d) => d !== '')
}
