import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

export interface PagesLogLine {
  step?: string
  stream: string
  line: string
  at?: string
}

export interface PagesDeployLog {
  deploy_id: string
  status: string
  running: boolean
  lines: PagesLogLine[]
}

/** Seeds the poll gap while a build runs; the server snapshot is cheap. */
const RUNNING_POLL_MS = 2000

export function pagesDeployLogQueryOptions(
  name: string,
  deployId: string,
  tail = 0,
) {
  return queryOptions({
    queryKey: [...appKeys.detail(name), 'pages-deploy-log', deployId, tail],
    queryFn: async (): Promise<PagesDeployLog> => {
      const qs = tail > 0 ? `?tail=${tail}` : ''
      const res = await fetch(
        `/api/v1/static-sites/${encodeURIComponent(name)}/deploys/${encodeURIComponent(deployId)}/log${qs}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `deploy log failed: ${res.status}`),
        )
      }
      return (await res.json()) as PagesDeployLog
    },
    refetchInterval: (q) => (q.state.data?.running ? RUNNING_POLL_MS : false),
  })
}
