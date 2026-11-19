// Pages (static sites) list with the optional fields the static-sites API
// grows into, plus the create call. Fields that the server does not send
// yet stay undefined and the UI shows a dash rather than a guess.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys, type StaticSite } from './staticSites'

export interface PagesSite extends StaticSite {
  last_deploy_at?: string
  size_bytes?: number
}

export const pagesSitesKeys = {
  list: [...staticSitesKeys.all, 'pages'] as const,
}

export async function fetchPagesSites(): Promise<PagesSite[]> {
  const res = await fetch('/api/v1/static-sites')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch static sites failed: ${res.status}`),
    )
  }
  return ((await res.json()) as PagesSite[] | null) ?? []
}

export function pagesSitesQueryOptions() {
  return queryOptions({
    queryKey: pagesSitesKeys.list,
    queryFn: fetchPagesSites,
    staleTime: 30_000,
  })
}

export function usePagesSites() {
  return useQuery(pagesSitesQueryOptions())
}

export type CreatePagesSiteResult = 'created' | 'fallback'

/** POST /api/v1/static-sites. A 404 or 405 means the server predates it: the caller then creates the site through the drop-publish path. */
export async function createPagesSite(
  name: string,
): Promise<CreatePagesSiteResult> {
  const res = await fetch('/api/v1/static-sites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (res.status === 404 || res.status === 405) return 'fallback'
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create site failed: ${res.status}`),
    )
  }
  return 'created'
}

export interface SiteDomainRecord {
  type: string
  name: string
  value: string
}

export interface SiteDomainSetup {
  domain: string
  mode: 'auto' | 'manual'
  records: SiteDomainRecord[]
  verified: boolean
  cert: string
  detail?: string
}

const siteBase = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/domains`

async function readSetup(
  res: Response,
  what: string,
): Promise<SiteDomainSetup> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as SiteDomainSetup
}

export async function addSiteDomain(
  name: string,
  domain: string,
): Promise<SiteDomainSetup> {
  const res = await fetch(siteBase(name), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domain }),
  })
  return readSetup(res, 'add domain')
}

export async function fetchSiteDomainStatus(
  name: string,
  domain: string,
): Promise<SiteDomainSetup> {
  return readSetup(
    await fetch(`${siteBase(name)}/${encodeURIComponent(domain)}/status`),
    'domain status',
  )
}

export interface PagesSiteDeploy {
  id: string
  status: string
  source: string
  commit_sha?: string
  started_at: string
  finished_at?: string
  error?: string
}

export interface PagesSiteDetail {
  name: string
  domains: string[]
  published: boolean
  file_count: number
  size_bytes: number
  updated_at?: string
  latest_deploy?: PagesSiteDeploy
}

const siteRoot = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}`

async function readJson<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function pagesSiteDetailQueryOptions(name: string) {
  return queryOptions({
    queryKey: [...staticSitesKeys.all, 'pages-detail', name] as const,
    queryFn: async () =>
      readJson<PagesSiteDetail>(await fetch(siteRoot(name)), 'site detail'),
    staleTime: 30_000,
    retry: false,
  })
}

/** Published size and latest deploy; `enabled` lets a list row skip it when the list already carried the values. */
export function usePagesSiteDetail(name: string, enabled = true) {
  return useQuery({ ...pagesSiteDetailQueryOptions(name), enabled })
}

export type CheckStatus = 'ok' | 'warn' | 'fail'

export interface SiteCheck {
  id: string
  status: CheckStatus
  title: string
  detail: string
  path?: string
}

export interface SiteChecksReport {
  name: string
  score: number
  files_scanned: number
  truncated: boolean
  total_bytes: number
  checks: SiteCheck[]
}

export const siteChecksKey = (name: string) =>
  [...staticSitesKeys.all, 'checks', name] as const

export function siteChecksQueryOptions(name: string) {
  return queryOptions({
    queryKey: siteChecksKey(name),
    queryFn: async () =>
      readJson<SiteChecksReport>(
        await fetch(`${siteRoot(name)}/checks`),
        'site checks',
      ),
    staleTime: 60_000,
    retry: false,
  })
}

export type TrafficWindow = '24h' | '7d'

export interface TrafficCount {
  label: string
  count: number
}

export interface SiteTraffic {
  name: string
  window: TrafficWindow
  since: string
  generated_at: string
  requests: number
  visitors: number
  bytes_out: number
  status: { '2xx': number; '3xx': number; '4xx': number; '5xx': number }
  top_paths: TrafficCount[]
  top_404_paths: TrafficCount[]
  /** Documents only, normalised; older servers omit these four. */
  top_pages?: TrafficCount[]
  assets?: { requests: number; bytes: number }
  broken_links?: TrafficCount[]
  probe_requests?: TrafficCount[]
  partial: boolean
  partial_notes?: string[]
}

/** null means the server has no edge access source (501), shown as an unavailable note rather than an error. */
export function siteTrafficQueryOptions(name: string, window: TrafficWindow) {
  return queryOptions({
    queryKey: [...staticSitesKeys.all, 'traffic', name, window] as const,
    queryFn: async (): Promise<SiteTraffic | null> => {
      const res = await fetch(`${siteRoot(name)}/traffic?window=${window}`)
      if (res.status === 501) return null
      return readJson<SiteTraffic>(res, 'site traffic')
    },
    staleTime: 60_000,
    retry: false,
  })
}

export type TrailingSlash = '' | 'off' | 'add' | 'remove'

export interface SiteSettings {
  name: string
  build_command: string
  output_dir: string
  spa_fallback: boolean | null
  clean_urls: boolean | null
  trailing_slash: TrailingSlash
  not_found_page: string
  effective: {
    spa?: boolean
    clean_urls?: boolean
    trailing_slash?: string
    not_found?: string
  }
}

export type SiteSettingKey =
  | 'build_command'
  | 'output_dir'
  | 'spa_fallback'
  | 'clean_urls'
  | 'trailing_slash'
  | 'not_found_page'

export interface SiteSettingsPatch {
  build_command?: string
  output_dir?: string
  spa_fallback?: boolean
  clean_urls?: boolean
  trailing_slash?: TrailingSlash
  not_found_page?: string
  reset?: SiteSettingKey[]
}

export const siteSettingsKey = (name: string) =>
  [...staticSitesKeys.all, 'settings', name] as const

export function siteSettingsQueryOptions(name: string) {
  return queryOptions({
    queryKey: siteSettingsKey(name),
    queryFn: async () =>
      readJson<SiteSettings>(
        await fetch(`${siteRoot(name)}/settings`),
        'site settings',
      ),
    staleTime: 30_000,
    retry: false,
  })
}

export function useSiteSettings(name: string) {
  return useQuery(siteSettingsQueryOptions(name))
}

export function useUpdateSiteSettings(name: string) {
  const qc = useQueryClient()
  return useMutation<SiteSettings, ApiError, SiteSettingsPatch>({
    mutationFn: async (patch) =>
      readJson<SiteSettings>(
        await fetch(`${siteRoot(name)}/settings`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(patch),
        }),
        'save settings',
      ),
    onSuccess: (data) => qc.setQueryData(siteSettingsKey(name), data),
  })
}

export type RuleSource = 'ui' | 'app.yaml' | 'file'

export interface UiRedirect {
  from: string
  to: string
  status?: number
  force?: boolean
}

export interface UiHeader {
  path: string
  set?: Record<string, string>
  remove?: string[]
}

export interface UiRules {
  redirects: UiRedirect[]
  headers: UiHeader[]
}

export interface EffectiveRedirect {
  from: string
  to: string
  status: number
  force: boolean
  source: RuleSource
  origin: string
}

export interface EffectiveHeader {
  path: string
  set: Record<string, string>
  remove: string[]
  source: RuleSource
  origin: string
}

export interface SiteRules {
  redirects: EffectiveRedirect[]
  headers: EffectiveHeader[]
  options: Record<string, unknown>
  problems: {
    source: string
    line?: number
    severity: 'error' | 'warning'
    message: string
  }[]
  published: boolean
  ui: UiRules
}

/** One 422 problem; index -1 is a cap breach that belongs to no single row. */
export interface RuleSaveProblem {
  kind: 'redirect' | 'header'
  index: number
  message: string
}

export class RulesSaveError extends ApiError {
  readonly problems: RuleSaveProblem[]
  constructor(message: string, problems: RuleSaveProblem[]) {
    super(422, message)
    this.problems = problems
  }
}

export const siteRulesKey = (name: string) =>
  [...staticSitesKeys.all, 'rules', name] as const

export function siteRulesQueryOptions(name: string) {
  return queryOptions({
    queryKey: siteRulesKey(name),
    queryFn: async () =>
      readJson<SiteRules>(await fetch(`${siteRoot(name)}/rules`), 'site rules'),
    staleTime: 15_000,
    retry: false,
  })
}

export function useSiteRules(name: string) {
  return useQuery(siteRulesQueryOptions(name))
}

async function putRules(name: string, body: UiRules): Promise<void> {
  const res = await fetch(`${siteRoot(name)}/rules`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (res.ok) return
  if (res.status === 422) {
    const j = (await res.json().catch(() => null)) as {
      error?: string
      problems?: RuleSaveProblem[]
    } | null
    throw new RulesSaveError(j?.error ?? 'invalid rules', j?.problems ?? [])
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `save rules failed: ${res.status}`),
  )
}

/** Shows the new UI rules at once and restores the previous ones when the server refuses them. */
export function useSaveSiteRules(name: string) {
  const qc = useQueryClient()
  const key = siteRulesKey(name)
  return useMutation<void, ApiError, UiRules, { prev: SiteRules | undefined }>({
    mutationFn: (body) => putRules(name, body),
    onMutate: async (body) => {
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData<SiteRules>(key)
      if (prev) qc.setQueryData<SiteRules>(key, { ...prev, ui: body })
      return { prev }
    },
    onError: (_e, _b, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })
}
