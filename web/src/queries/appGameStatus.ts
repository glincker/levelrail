// Matches internal/api/game_status.go: one ping of a game server's own port.
import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { pollUnlessMissing } from '../lib/pollUnlessMissing'

export type GameExposure =
  'stream' | 'direct' | 'published' | 'private' | 'remote_node' | 'none'

export interface GameStatus {
  app: string
  exposure: GameExposure
  running: boolean
  online: boolean
  kind?: 'minecraft' | 'tcp'
  version?: string
  players: number
  max_players: number
  latency_ms: number
  motd?: string
  join_address?: string
  join_port?: number
  checked_at: string
}

// Slow on purpose: the server also caches an answer for a few seconds.
export const GAME_STATUS_POLL_MS = 30_000

export const appGameStatusKeys = {
  detail: (app: string) => ['apps', app, 'game-status'] as const,
}

export async function fetchGameStatus(app: string): Promise<GameStatus> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(app)}/game-status`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch game status failed: ${res.status}`),
    )
  }
  return (await res.json()) as GameStatus
}

export function appGameStatusQueryOptions(app: string) {
  return queryOptions({
    queryKey: appGameStatusKeys.detail(app),
    queryFn: () => fetchGameStatus(app),
  })
}

// react-query pauses the interval while the tab is hidden.
export function useGameStatus(app: string) {
  return useQuery({
    ...appGameStatusQueryOptions(app),
    refetchInterval: pollUnlessMissing(GAME_STATUS_POLL_MS),
    refetchIntervalInBackground: false,
  })
}
