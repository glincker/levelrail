import { useCallback, useEffect, useRef, useState } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import type { Deployment, DeploymentEvent } from '../types/deployment'
import type { DeploymentFilters } from '../lib/deploymentFilters'
import {
  applyEventToLane,
  applyEventToPages,
  prependToPages,
  replaceInPages,
  type DeploymentPages,
} from '../lib/deploymentsCache'
import { deploymentKeys } from '../queries/deployments'

export type DeploymentsStreamState = 'connecting' | 'live' | 'reconnecting'

const STREAM_URL = '/api/v1/deployments/stream'
const SUMMARY_DEBOUNCE_MS = 2000
const RETRY_BASE_MS = 1000
const RETRY_MAX_MS = 30_000

const EVENT_TYPES = new Set(['created', 'step', 'finished'])

export function parseDeploymentEvent(raw: string): DeploymentEvent | null {
  try {
    const v = JSON.parse(raw) as Partial<DeploymentEvent> | null
    if (
      v &&
      typeof v.type === 'string' &&
      EVENT_TYPES.has(v.type) &&
      v.deployment &&
      typeof v.deployment.id === 'string'
    ) {
      return v as DeploymentEvent
    }
  } catch {
    return null
  }
  return null
}

/** Patches every cached deployments list in place and returns the row to hold back for the current one. */
export function patchCaches(
  qc: QueryClient,
  ev: DeploymentEvent,
  current: DeploymentFilters,
  currentKey: readonly unknown[],
  atTop: boolean,
): Deployment | null {
  let pending: Deployment | null = null
  const currentJson = JSON.stringify(currentKey)
  for (const q of qc
    .getQueryCache()
    .findAll({ queryKey: deploymentKeys.lists() })) {
    const data = q.state.data as DeploymentPages | undefined
    if (!data) continue
    if (JSON.stringify(q.queryKey) === currentJson) {
      const out = applyEventToPages(data, ev, current, atTop)
      pending = out.pending
      if (out.data !== data) qc.setQueryData(q.queryKey, out.data)
    } else {
      const next = replaceInPages(data, ev.deployment)
      if (next !== data) qc.setQueryData(q.queryKey, next)
    }
  }
  qc.setQueryData<Deployment[]>(deploymentKeys.lane(), (rows) =>
    rows ? applyEventToLane(rows, ev) : rows,
  )
  return pending
}

export interface DeploymentsStream {
  state: DeploymentsStreamState
  pending: Deployment[]
  flushPending: () => void
}

export function useDeploymentsStream(
  filters: DeploymentFilters,
  listKey: readonly unknown[],
  isAtTop: () => boolean,
): DeploymentsStream {
  const qc = useQueryClient()
  const [state, setState] = useState<DeploymentsStreamState>('connecting')
  const [pending, setPending] = useState<Deployment[]>([])
  const live = useRef({ filters, listKey, isAtTop })

  useEffect(() => {
    live.current = { filters, listKey, isAtTop }
  })
  const keyJson = JSON.stringify(listKey)
  const [pendingKey, setPendingKey] = useState(keyJson)
  if (pendingKey !== keyJson) {
    setPendingKey(keyJson)
    setPending([])
  }

  useEffect(() => {
    let source: EventSource | null = null
    let retry: ReturnType<typeof setTimeout> | undefined
    let summaryTimer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let hadError = false
    let stopped = false

    const scheduleSummary = () => {
      if (summaryTimer) return
      summaryTimer = setTimeout(() => {
        summaryTimer = undefined
        void qc.invalidateQueries({ queryKey: deploymentKeys.summary() })
      }, SUMMARY_DEBOUNCE_MS)
    }

    const connect = () => {
      if (stopped) return
      const es = new EventSource(STREAM_URL)
      source = es
      es.onopen = () => {
        attempt = 0
        setState('live')
        if (hadError) {
          hadError = false
          void qc.invalidateQueries({ queryKey: deploymentKeys.all })
        }
      }
      es.onmessage = (m: MessageEvent<string>) => {
        const ev = parseDeploymentEvent(m.data)
        if (!ev) return
        const cur = live.current
        const held = patchCaches(
          qc,
          ev,
          cur.filters,
          cur.listKey,
          cur.isAtTop(),
        )
        if (held) {
          setPending((p) =>
            p.some((r) => r.id === held.id)
              ? p.map((r) => (r.id === held.id ? held : r))
              : [held, ...p],
          )
        }
        if (ev.type !== 'step') scheduleSummary()
      }
      es.onerror = () => {
        hadError = true
        setState('reconnecting')
        if (es.readyState === EventSource.CLOSED) {
          es.close()
          const delay = Math.min(RETRY_BASE_MS * 2 ** attempt, RETRY_MAX_MS)
          attempt += 1
          retry = setTimeout(connect, delay)
        }
      }
    }
    connect()
    return () => {
      stopped = true
      source?.close()
      clearTimeout(retry)
      clearTimeout(summaryTimer)
    }
  }, [qc])

  const flushPending = useCallback(() => {
    const rows = [...pending].sort((a, b) =>
      b.started_at.localeCompare(a.started_at),
    )
    qc.setQueryData<DeploymentPages>(live.current.listKey, (data) =>
      data ? prependToPages(data, rows) : data,
    )
    setPending([])
  }, [pending, qc])

  return { state, pending, flushPending }
}
