// Routing rules of a static site: GET /api/v1/apps/{name}/static-routes
// (internal/api/static_routes.go), plus the test-a-URL and lint calls.

import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface StaticRule {
  source: string
  from: string
  to: string
  status: number
  force?: boolean
  query?: { key: string; value: string }[]
}

export interface StaticHeaderBlock {
  source: string
  path: string
  set?: { name: string; value: string }[]
  remove?: string[]
}

export interface StaticOptions {
  spa?: boolean
  clean_urls?: boolean
  trailing_slash?: string
  not_found?: string
}

export interface StaticRouteProblem {
  source: string
  line?: number
  severity: 'error' | 'warning'
  message: string
}

export interface StaticRoutes {
  version: number
  rules: StaticRule[]
  headers: StaticHeaderBlock[]
  options: StaticOptions
  problems: StaticRouteProblem[]
  published: boolean
}

export interface StaticRouteDecision {
  kind: 'serve' | 'redirect' | 'not_found'
  status: number
  location?: string
  file?: string
  rule?: string
  headers: { name: string; value: string }[]
  remove_headers: string[]
  trace: string[]
}

export interface StaticRoutesLint {
  rules: StaticRule[]
  headers: StaticHeaderBlock[]
  problems: StaticRouteProblem[]
  error?: string
}

export const staticRoutesKeys = {
  app: (app: string) => ['static-routes', app] as const,
}

const base = (app: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/static-routes`

async function send<T>(url: string, what: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method: body === undefined ? 'GET' : 'POST',
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useStaticRoutes(app: string) {
  return useQuery({
    queryKey: staticRoutesKeys.app(app),
    queryFn: () => send<StaticRoutes>(base(app), 'fetch routing rules'),
    retry: false,
  })
}

export function useTestStaticRoutes(app: string) {
  return useMutation<
    StaticRouteDecision,
    ApiError,
    { path: string; query: string }
  >({
    mutationFn: (req) =>
      send<StaticRouteDecision>(`${base(app)}/test`, 'test routing rules', req),
  })
}

export function useLintStaticRoutes(app: string) {
  return useMutation<
    StaticRoutesLint,
    ApiError,
    { redirects: string; headers: string }
  >({
    mutationFn: (req) =>
      send<StaticRoutesLint>(`${base(app)}/lint`, 'check routing rules', req),
  })
}
