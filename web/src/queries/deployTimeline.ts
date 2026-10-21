// Fleet deploy timeline: GET /api/v1/deployments/timeline (windowed, cursor
// paginated) with in-flight rows patched live from the deployments SSE stream.

import { useEffect, useMemo, useState } from 'react'
import { infiniteQueryOptions, useInfiniteQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { parseDeploymentEvent } from '../hooks/useDeploymentsStream'
import {
  applyStreamedDeploy,
  toQuery,
  ZOOM_MS,
  type TimelineAlert,
  type TimelineDeploy,
  type TimelineFilters,
  type TimelinePage,
} from '../lib/deployTimeline'
import type { Deployment } from '../types/deployment'
import { setVisibleInterval } from '../lib/pageVisibility'

export const deployTimelineKeys = {
  all: ['deploy-timeline'] as const,
  list: (f: TimelineFilters) => [...deployTimelineKeys.all, f] as const,
}

const STREAM_URL = '/api/v1/deployments/stream'
const RETRY_MS = 5000
const MAX_EVENTS = 499
const NOW_TICK_MS = 15_000

async function fetchTimelinePage(
  f: TimelineFilters,
  cursor: string,
): Promise<TimelinePage> {
  const res = await fetch(`/api/v1/deployments/timeline?${toQuery(f, cursor)}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `deploy timeline failed: ${String(res.status)}`,
      ),
    )
  }
  return (await res.json()) as TimelinePage
}

export function deployTimelineOptions(f: TimelineFilters) {
  return infiniteQueryOptions({
    queryKey: deployTimelineKeys.list(f),
    queryFn: ({ pageParam }) => fetchTimelinePage(f, pageParam),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}

export type TimelineStreamState = 'connecting' | 'live' | 'reconnecting'

function useStreamedDeploys(resetKey: string): {
  events: Deployment[]
  state: TimelineStreamState
} {
  const [events, setEvents] = useState<Deployment[]>([])
  const [state, setState] = useState<TimelineStreamState>('connecting')
  const [seenKey, setSeenKey] = useState(resetKey)
  if (seenKey !== resetKey) {
    setSeenKey(resetKey)
    setEvents([])
  }

  useEffect(() => {
    let source: EventSource | null = null
    let retry: ReturnType<typeof setTimeout> | undefined
    let stopped = false
    const connect = () => {
      if (stopped) return
      const es = new EventSource(STREAM_URL)
      source = es
      es.onopen = () => {
        setState('live')
      }
      es.onmessage = (m: MessageEvent<string>) => {
        const ev = parseDeploymentEvent(m.data)
        if (ev) {
          setEvents((prev) => [...prev.slice(-MAX_EVENTS), ev.deployment])
        }
      }
      es.onerror = () => {
        es.close()
        setState('reconnecting')
        retry = setTimeout(connect, RETRY_MS)
      }
    }
    connect()
    return () => {
      stopped = true
      if (retry) clearTimeout(retry)
      source?.close()
    }
  }, [])

  return { events, state }
}

export function useDeployTimeline(filters: TimelineFilters) {
  const query = useInfiniteQuery(deployTimelineOptions(filters))
  const key = JSON.stringify(filters)
  const { events, state } = useStreamedDeploys(key)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), NOW_TICK_MS)
    return () => {
      stop()
    }
  }, [])

  const windowEnd = now
  const windowStart = windowEnd - ZOOM_MS[filters.zoom]
  const fetched = query.data
  const items = useMemo<TimelineDeploy[]>(() => {
    let rows = fetched?.pages.flatMap((p) => p.items) ?? []
    for (const ev of events) {
      rows = applyStreamedDeploy(rows, ev, filters, windowStart)
    }
    return rows
  }, [fetched, events, filters, windowStart])

  const alerts = useMemo<TimelineAlert[]>(
    () => fetched?.pages[0]?.alerts ?? [],
    [fetched],
  )

  return { query, items, alerts, streamState: state, windowStart, windowEnd }
}
