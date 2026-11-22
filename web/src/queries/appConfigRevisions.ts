// Matches internal/api/app_config_revisions.go: config history and revert.
import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { appKeys } from './apps'
import { timelineKeys } from './appTimeline'
import { useApiMutation } from './useApiMutation'

export interface ConfigFieldChange {
  kind: string
  field: string
  from?: string
  to?: string
}

export interface ConfigRevision {
  n: number
  at: string
  actor: string
  kinds: string[]
  changes: ConfigFieldChange[]
  revert_of?: number
}

export interface RevertResult {
  app: string
  reverted_to: number
  revision?: number
  dry_run?: boolean
  changes: ConfigFieldChange[]
  skipped?: string[]
  pending_apply: boolean
}

const MIN_WINDOW = '1s'

export const configRevisionKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'config-revisions'] as const,
}

export function useConfigRevisions(appName: string) {
  return useQuery({
    queryKey: configRevisionKeys.list(appName),
    queryFn: async () => {
      const res = await apiRequest<{ revisions?: ConfigRevision[] | null }>(
        `/api/v1/apps/${encodeURIComponent(appName)}/changes?window=${MIN_WINDOW}`,
        { fallbackMessage: 'fetch config revisions failed' },
      )
      return res.revisions ?? []
    },
    retry: false,
  })
}

export function useRevertConfig(appName: string) {
  return useApiMutation<RevertResult, { to: number; dryRun: boolean }>({
    fn: ({ to, dryRun }) =>
      apiRequest<RevertResult>(
        `/api/v1/apps/${encodeURIComponent(appName)}/revert`,
        {
          method: 'POST',
          body: { to, dry_run: dryRun },
          fallbackMessage: 'revert config failed',
        },
      ),
    invalidate: (vars) =>
      vars.dryRun
        ? []
        : [
            configRevisionKeys.list(appName),
            appKeys.detail(appName),
            timelineKeys.list(appName),
          ],
    meta: { errorToast: false },
  })
}

// The revision in force at a moment: the newest one recorded at or before it.
export function revisionAt(
  revisions: readonly ConfigRevision[],
  at: string,
): ConfigRevision | undefined {
  const t = Date.parse(at)
  return revisions.find((r) => Date.parse(r.at) <= t)
}
