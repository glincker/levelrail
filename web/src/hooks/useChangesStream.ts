import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  invalidateChangeTopic,
  isChangeTopic,
  type ChangeTopic,
} from '../queries/changeTopics'
import { setChangeStreamHealthy } from '../lib/pollTier'

const STREAM_URL = '/api/v1/events/changes'
const RETRY_BASE_MS = 1000
const RETRY_MAX_MS = 30_000
const WATCHDOG_FACTOR = 2.5

/** Reads the heartbeat the server announces in each ping, null when absent. */
export function parseHeartbeatMs(raw: string): number | null {
  try {
    const v = JSON.parse(raw) as { heartbeat_ms?: unknown } | null
    const ms = v?.heartbeat_ms
    return typeof ms === 'number' && Number.isFinite(ms) && ms > 0 ? ms : null
  } catch {
    return null
  }
}

/** Returns the known topics of one stream message, ignoring anything else. */
export function parseChangeTopics(raw: string): ChangeTopic[] {
  try {
    const v = JSON.parse(raw) as { topics?: unknown } | null
    if (!v || !Array.isArray(v.topics)) return []
    return v.topics.filter(isChangeTopic)
  } catch {
    return []
  }
}

/** Keeps one change stream open while signed in; topics invalidate their mapped list keys. */
export function useChangesStream(enabled: boolean): void {
  const qc = useQueryClient()

  useEffect(() => {
    if (!enabled || typeof EventSource === 'undefined') return
    let source: EventSource | null = null
    let retry: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    let stopped = false
    let watchdog: ReturnType<typeof setTimeout> | undefined
    let heartbeatMs: number | null = null

    const reconnect = (es: EventSource) => {
      clearTimeout(watchdog)
      setChangeStreamHealthy(false)
      es.close()
      source = null
      const delay = Math.min(RETRY_BASE_MS * 2 ** attempt, RETRY_MAX_MS)
      attempt += 1
      retry = setTimeout(connect, delay)
    }
    // A stream that goes silent past 2.5 heartbeats is treated as dead.
    const arm = (es: EventSource) => {
      clearTimeout(watchdog)
      if (heartbeatMs === null) return
      watchdog = setTimeout(() => reconnect(es), heartbeatMs * WATCHDOG_FACTOR)
    }

    const connect = () => {
      if (stopped || document.visibilityState === 'hidden') return
      const es = new EventSource(STREAM_URL)
      source = es
      es.onopen = () => {
        attempt = 0
        setChangeStreamHealthy(true)
      }
      es.addEventListener('ping', (m: MessageEvent<string>) => {
        heartbeatMs = parseHeartbeatMs(m.data) ?? heartbeatMs
        arm(es)
      })
      es.onmessage = (m: MessageEvent<string>) => {
        arm(es)
        for (const topic of parseChangeTopics(m.data)) {
          invalidateChangeTopic(qc, topic)
        }
      }
      es.onerror = () => {
        setChangeStreamHealthy(false)
        if (es.readyState === EventSource.CLOSED) reconnect(es)
      }
    }
    connect()
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') {
        clearTimeout(retry)
        clearTimeout(watchdog)
        source?.close()
        source = null
        setChangeStreamHealthy(false)
      } else if (!source && !stopped) {
        attempt = 0
        connect()
      }
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      stopped = true
      clearTimeout(retry)
      clearTimeout(watchdog)
      source?.close()
      setChangeStreamHealthy(false)
    }
  }, [enabled, qc])
}
