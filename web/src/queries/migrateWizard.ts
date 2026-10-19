// Migration plan, selective import and cutover checklist
// (internal/api/migration_wizard.go). The source token only travels in the
// request body of the plan and import calls and is never put in the cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type MigrateVerdict =
  | 'will_import'
  | 'imports_with_changes'
  | 'needs_manual'
  | 'unsupported'
  | 'already_imported'

export type MigrateFixKind = 'auto' | 'guided' | 'validate'

export type MigrateFixCode =
  | 'rename'
  | 'set_port'
  | 'hold_domains'
  | 'check_dns'
  | 'copy_data'
  | 'copy_volumes'
  | 'use_template'
  | 'connect_repo'
  | 'deploy_compose'

export interface MigrateFix {
  code: MigrateFixCode
  kind: MigrateFixKind
  detail?: string
}

export interface MigrateVolume {
  name?: string
  host_path?: string
  container_path: string
  size_bytes?: number
}

export interface MigratePreview {
  name: string
  image?: string
  engine?: string
  version?: string
  port?: number
  domains?: string[]
  env: number
  secrets: number
  volumes: number
  replicas?: number
  dropped_keys?: string[]
}

export interface MigrateItem {
  kind: string
  source_id: string
  name: string
  target?: string
  project?: string
  environment?: string
  server?: string
  source?: string
  verdict: MigrateVerdict
  reasons?: string[]
  manual?: string[]
  domains?: string[]
  port?: number
  env_count: number
  volumes?: MigrateVolume[]
  crons?: string[]
  fixes?: MigrateFix[]
  preview?: MigratePreview
}

export function isSelectableItem(item: MigrateItem): boolean {
  return item.verdict !== 'unsupported' && item.verdict !== 'already_imported'
}

export interface MigrateReport {
  platform: string
  items: MigrateItem[]
  counts: Partial<Record<MigrateVerdict, number>>
  notes?: string[]
}

export interface MigrateOverride {
  name?: string
  port?: number
  drop_domains?: boolean
}

export type MigratePlatform = 'coolify' | 'dokploy' | 'caprover'

export interface MigrateSource {
  platform: MigratePlatform
  url: string
  token: string
  insecure_tls?: boolean
}

export interface MigrateRequest extends MigrateSource {
  only?: string[]
  overrides?: Record<string, MigrateOverride>
  dry_run?: boolean
}

export type StepId =
  | 'imported'
  | 'data_verified'
  | 'shadow_running'
  | 'dns_switched'
  | 'old_stopped'

export interface ChecklistStep {
  id: StepId
  status: 'pending' | 'done'
  at?: string
  actor?: string
  note?: string
  evidence?: string[]
}

export interface ChecklistVolume {
  name: string
  source_command?: string
  target_command: string
}

export interface MigrateChecklist {
  app: string
  platform?: string
  source_id?: string
  steps: ChecklistStep[]
  current: StepId | ''
  progress: number
  total: number
  domains?: string[]
  volumes?: ChecklistVolume[]
  tracked: boolean
}

export interface VolumeStat {
  name: string
  source_files: number
  source_bytes: number
  target_files: number
  target_bytes: number
}

export interface StepRequest {
  action: 'complete' | 'reset'
  note?: string
  confirm?: boolean
  databases?: string[]
  volumes?: VolumeStat[]
}

export class StepBlockedError extends Error {
  problems: string[]
  constructor(message: string, problems: string[]) {
    super(message)
    this.problems = problems
  }
}

async function post<T>(url: string, body: unknown, what: string): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function usePlanMigration() {
  return useMutation({
    mutationFn: (req: MigrateRequest) =>
      post<MigrateReport>('/api/v1/migration/plan', req, 'plan'),
  })
}

export function useImportMigration() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: MigrateRequest) =>
      post<MigrateReport>('/api/v1/migration/import', req, 'import'),
    onSuccess: (_report, req) => {
      if (!req.dry_run) {
        void qc.invalidateQueries({ queryKey: ['migrate-checklists'] })
      }
    },
  })
}

export function useMigrateChecklists() {
  return useQuery({
    queryKey: ['migrate-checklists'],
    queryFn: async ({ signal }): Promise<MigrateChecklist[]> => {
      const res = await fetch('/api/v1/migration/checklists', {
        signal,
        cache: 'no-store',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `load checklists failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { checklists: MigrateChecklist[] }
      return body.checklists
    },
  })
}

export function useStepChecklist() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (vars: {
      app: string
      step: string
      req: StepRequest
    }): Promise<MigrateChecklist> => {
      const res = await fetch(
        `/api/v1/migration/checklists/${encodeURIComponent(vars.app)}/steps/${encodeURIComponent(vars.step)}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(vars.req),
        },
      )
      if (res.status === 409) {
        const body = (await res.json()) as {
          error: string
          problems?: string[]
        }
        throw new StepBlockedError(body.error, body.problems ?? [])
      }
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `update step failed: ${res.status}`),
        )
      }
      return (await res.json()) as MigrateChecklist
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['migrate-checklists'] })
    },
  })
}

export interface CutoverDomainCheck {
  domain: string
  verdict: string
}

export async function fetchCutoverVerdict(
  app: string,
): Promise<{ verdict: string; domains: CutoverDomainCheck[] }> {
  const res = await fetch(
    `/api/v1/migration/cutover?app=${encodeURIComponent(app)}`,
    { cache: 'no-store' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `dns check failed: ${res.status}`),
    )
  }
  return (await res.json()) as {
    verdict: string
    domains: CutoverDomainCheck[]
  }
}
