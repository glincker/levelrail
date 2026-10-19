import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

export interface EnvFinding {
  kind: string
  key: string
  severity: 'warning' | 'error'
  detail?: string
  fix: { type: 'auto' | 'manual'; action: string }
}

export interface EnvChange {
  id: number
  at: string
  actor: string
  key: string
  action: 'added' | 'changed' | 'removed'
  source: string
}

export const envHealthKeys = {
  check: (app: string) => ['envHealth', 'check', app] as const,
  history: (app: string) => ['envHealth', 'history', app] as const,
}

const appPath = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}`

async function readJSON<T>(res: Response, fallback: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
  return (await res.json()) as T
}

export function useEnvCheck(app: string) {
  return useQuery({
    queryKey: envHealthKeys.check(app),
    queryFn: async (): Promise<EnvFinding[]> => {
      const res = await fetch(`${appPath(app)}/env/check`)
      const body = await readJSON<{ findings: EnvFinding[] }>(
        res,
        `check env failed: ${String(res.status)}`,
      )
      return body.findings
    },
  })
}

export function useEnvHistory(app: string, limit = 10) {
  return useQuery({
    queryKey: [...envHealthKeys.history(app), limit],
    queryFn: async (): Promise<EnvChange[]> => {
      const res = await fetch(
        `${appPath(app)}/env/history?limit=${String(limit)}`,
      )
      const body = await readJSON<{ changes: EnvChange[] }>(
        res,
        `load env history failed: ${String(res.status)}`,
      )
      return body.changes
    },
  })
}

export function useFixEnvFinding(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (f: Pick<EnvFinding, 'kind' | 'key'>) => {
      const res = await fetch(`${appPath(app)}/env/check/fix`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: f.kind, key: f.key }),
      })
      return readJSON<{ findings: EnvFinding[] }>(
        res,
        `fix env failed: ${String(res.status)}`,
      )
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: envHealthKeys.check(app) })
      void qc.invalidateQueries({ queryKey: envHealthKeys.history(app) })
      void qc.invalidateQueries({ queryKey: appKeys.all })
    },
  })
}
