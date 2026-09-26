// Fetchers and hooks for the declarative resource endpoints
// (internal/api/iac.go): POST /api/v1/apply/plan, POST /api/v1/apply and
// GET /api/v1/export.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { projectKeys } from './projects'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type IacAction = 'create' | 'update' | 'delete' | 'noop' | 'error'
export type IacFieldOp = 'add' | 'change' | 'remove' | 'keep'

export interface IacFieldChange {
  path: string
  op: IacFieldOp
  old?: string
  new?: string
}

export interface IacChange {
  kind: string
  name: string
  scope?: string
  action: IacAction
  fields?: IacFieldChange[]
  kept?: IacFieldChange[]
  warnings?: string[]
  reason?: string
  denied?: boolean
  file?: string
  line?: number
}

export interface IacSummary {
  create: number
  update: number
  delete: number
  noop: number
  error: number
}

export interface IacPlan {
  source?: string
  prune?: boolean
  changes: IacChange[]
  summary: IacSummary
  hash: string
}

export interface IacIssue {
  file?: string
  path?: string
  line: number
  message: string
}

export type IacItemStatus = 'applied' | 'failed' | 'denied' | 'skipped' | 'noop'

export interface IacItemResult extends IacChange {
  status: IacItemStatus
  error?: string
}

export interface IacApplyResult {
  plan: IacPlan
  results: IacItemResult[]
  applied: number
  failed: number
  skipped: number
}

export interface IacFile {
  name: string
  content: string
}

export interface IacRequest {
  files: IacFile[]
  source?: string
  project?: string
  prune?: boolean
  no_deploy?: boolean
  continue_on_error?: boolean
  expected_plan_hash?: string
}

export interface IacExportFile {
  name: string
  kind: string
  content: string
}

export interface IacExportResult {
  files: IacExportFile[]
  warnings?: string[]
}

// Thrown for a 422: the files did not validate, with file and line per issue.
export class IacIssuesError extends ApiError {
  readonly issues: IacIssue[]

  constructor(issues: IacIssue[]) {
    super(422, 'The resource files are invalid.')
    this.name = 'IacIssuesError'
    this.issues = issues
  }
}

async function iacRequest<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (res.status === 422) {
    const body = (await res.json().catch(() => null)) as {
      issues?: IacIssue[]
    } | null
    if (body?.issues?.length) throw new IacIssuesError(body.issues)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function postJson(body: IacRequest): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function usePlanIac() {
  return useMutation<IacPlan, ApiError, IacRequest>({
    mutationFn: async (req) => {
      const out = await iacRequest<{ plan: IacPlan }>(
        '/api/v1/apply/plan',
        postJson(req),
        'plan',
      )
      return out.plan
    },
  })
}

export function useApplyIac() {
  const queryClient = useQueryClient()
  return useMutation<IacApplyResult, ApiError, IacRequest>({
    mutationFn: async (req) => {
      const out = await iacRequest<{ result: IacApplyResult }>(
        '/api/v1/apply',
        postJson(req),
        'apply',
      )
      return out.result
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: appKeys.all })
      void queryClient.invalidateQueries({ queryKey: projectKeys.all })
    },
  })
}

export interface IacExportParams {
  project?: string
  app?: string
  includeEnvValues: boolean
}

export function useExportIac() {
  return useMutation<IacExportResult, ApiError, IacExportParams>({
    mutationFn: (params) => {
      const q = new URLSearchParams()
      if (params.project) q.set('project', params.project)
      if (params.app) q.set('app', params.app)
      q.set('include_env_values', String(params.includeEnvValues))
      return iacRequest<IacExportResult>(
        `/api/v1/export?${q.toString()}`,
        undefined,
        'export',
      )
    },
  })
}
