// Cross-app deployments: GET /api/v1/deployments (cursor paginated),
// /deployments/summary and the per-app cancel endpoint. Live patching of
// these caches lives in hooks/useDeploymentsStream.ts.

import {
  infiniteQueryOptions,
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  Deployment,
  DeploymentListPage,
  DeploymentsSummary,
} from '../types/deployment'
import {
  serverFilters,
  toApiParams,
  type DeploymentFilters,
} from '../lib/deploymentFilters'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { triggerDeploy } from './deploys'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'

export const PAGE_SIZE = 50
export const LANE_LIMIT = 50
const LOG_TAIL_LINES = 12

export const deploymentKeys = {
  all: ['deployments'] as const,
  lists: () => [...deploymentKeys.all, 'list'] as const,
  list: (f: DeploymentFilters) =>
    [...deploymentKeys.lists(), serverFilters(f)] as const,
  lane: () => [...deploymentKeys.all, 'lane'] as const,
  summary: () => [...deploymentKeys.all, 'summary'] as const,
  logTail: (app: string, id: string) =>
    [...deploymentKeys.all, 'log-tail', app, id] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  return (await res.json()) as T
}

export function fetchDeploymentPage(
  filters: DeploymentFilters,
  cursor: string,
): Promise<DeploymentListPage> {
  const params = toApiParams(filters, cursor, PAGE_SIZE)
  return getJson<DeploymentListPage>(
    `/api/v1/deployments?${params.toString()}`,
    'list deployments',
  )
}

export function deploymentsInfiniteOptions(filters: DeploymentFilters) {
  return infiniteQueryOptions({
    queryKey: deploymentKeys.list(filters),
    queryFn: ({ pageParam }) => fetchDeploymentPage(filters, pageParam),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}

export function deploymentsLaneOptions() {
  return queryOptions({
    queryKey: deploymentKeys.lane(),
    queryFn: async (): Promise<Deployment[]> => {
      const params = new URLSearchParams({
        status: 'building,queued',
        limit: String(LANE_LIMIT),
      })
      const page = await getJson<DeploymentListPage>(
        `/api/v1/deployments?${params.toString()}`,
        'list building deployments',
      )
      return page.items
    },
  })
}

export function deploymentsSummaryOptions() {
  return queryOptions({
    queryKey: deploymentKeys.summary(),
    queryFn: () =>
      getJson<DeploymentsSummary>(
        '/api/v1/deployments/summary',
        'deployments summary',
      ),
  })
}

export function parseLogTail(text: string, lines = LOG_TAIL_LINES): string[] {
  return text
    .split('\n')
    .map((l) => l.trimEnd())
    .filter((l) => l !== '')
    .slice(-lines)
}

export function deploymentLogTailOptions(app: string, id: string) {
  return queryOptions({
    queryKey: deploymentKeys.logTail(app, id),
    queryFn: async (): Promise<string[]> => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/deploys/${encodeURIComponent(id)}/logs/download`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          `fetch deploy log failed: ${String(res.status)}`,
        )
      }
      return parseLogTail(await res.text())
    },
    staleTime: 60_000,
    retry: false,
  })
}

export function deployLogsPath(d: Pick<Deployment, 'app' | 'id'>) {
  return {
    to: '/apps/$name/deploys/$deployId/logs' as const,
    params: { name: d.app, deployId: d.id },
  }
}

export function isCancelUnsupported(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 405)
  )
}

export async function cancelDeployment(d: Deployment): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(d.app)}/deploys/${encodeURIComponent(d.id)}/cancel`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `cancel deploy failed: ${String(res.status)}`,
      ),
    )
  }
}

export interface DeployImageInput {
  app: string
  image: string
}

export function useDeploymentMutations() {
  const qc = useQueryClient()
  const refresh = (app: string) => {
    void qc.invalidateQueries({ queryKey: deploymentKeys.all })
    void qc.invalidateQueries({ queryKey: deployAttemptKeys.list(app) })
    void qc.invalidateQueries({ queryKey: appKeys.detail(app) })
  }
  const deployImage = useMutation({
    mutationFn: ({ app, image }: DeployImageInput) =>
      triggerDeploy(app, { image, confirm: true }),
    onSuccess: (_r, v) => {
      refresh(v.app)
    },
  })
  const cancel = useMutation({
    mutationFn: cancelDeployment,
    onSuccess: (_r, d) => {
      refresh(d.app)
    },
  })
  return { deployImage, cancel }
}
