// Live view of the domain watcher (GET /api/v1/domains/watch, SSE): which
// hostnames are still waiting for DNS or a certificate and when the server
// checks next. One stream is shared by every component on the page.

import { useEffect, useRef, useSyncExternalStore } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { domainKeys } from './domains'

export type WatchState =
  'waiting_dns' | 'propagating' | 'waiting_certificate' | 'live' | 'stalled'

export interface WatchEntry {
  domain: string
  app?: string
  state: WatchState
  since?: string
  last_checked?: string
  next_check?: string
  attempts?: number
  stalled_at?: string
  stopped?: boolean
  notified_at?: string
}

export type WatchKind = 'snapshot' | 'update' | 'removed'

export interface WatchMessage {
  kind: WatchKind
  entry: WatchEntry
}

const STATES: readonly string[] = [
  'waiting_dns',
  'propagating',
  'waiting_certificate',
  'live',
  'stalled',
]

export function parseWatchMessage(data: string): WatchMessage | null {
  let raw: unknown
  try {
    raw = JSON.parse(data)
  } catch {
    return null
  }
  if (typeof raw !== 'object' || raw === null) return null
  const { kind, entry } = raw as { kind?: unknown; entry?: unknown }
  if (kind !== 'snapshot' && kind !== 'update' && kind !== 'removed')
    return null
  if (typeof entry !== 'object' || entry === null) return null
  const e = entry as Partial<WatchEntry>
  if (typeof e.domain !== 'string' || e.domain === '') return null
  // An unknown state from a newer server is shown as nothing, not guessed.
  if (typeof e.state !== 'string' || !STATES.includes(e.state)) {
    if (kind !== 'removed') return null
  }
  return { kind, entry: e as WatchEntry }
}

export type WatchMap = ReadonlyMap<string, WatchEntry>

export function applyWatchMessage(map: WatchMap, msg: WatchMessage): WatchMap {
  const next = new Map(map)
  const key = msg.entry.domain.toLowerCase()
  if (msg.kind === 'removed') next.delete(key)
  else next.set(key, msg.entry)
  return next
}

export type WatchStatus = 'idle' | 'connecting' | 'live' | 'unavailable'

export interface WatchSnapshot {
  entries: WatchMap
  status: WatchStatus
}

const IDLE: WatchSnapshot = { entries: new Map(), status: 'idle' }
const BACKOFF_START_MS = 1_000
const BACKOFF_MAX_MS = 30_000

export interface WatchStore {
  subscribe: (cb: () => void) => () => void
  getSnapshot: () => WatchSnapshot
}

// Refcounted: the stream opens with the first subscriber and closes with the
// last. A non-2xx answer (EventSource goes CLOSED before ever opening) means
// the server has no watcher, so the store gives up quietly.
export function createWatchStore(
  open: () => EventSource,
  setTimer: typeof setTimeout = setTimeout,
  clearTimer: typeof clearTimeout = clearTimeout,
): WatchStore {
  let snap = IDLE
  const listeners = new Set<() => void>()
  let source: EventSource | null = null
  let timer: ReturnType<typeof setTimeout> | null = null
  let delay = BACKOFF_START_MS

  function set(next: WatchSnapshot) {
    snap = next
    listeners.forEach((l) => {
      l()
    })
  }

  function connect() {
    if (typeof EventSource === 'undefined') {
      set({ entries: new Map(), status: 'unavailable' })
      return
    }
    const es = open()
    source = es
    let opened = false
    set({ ...snap, status: 'connecting' })
    es.onopen = () => {
      opened = true
      delay = BACKOFF_START_MS
      set({ ...snap, status: 'live' })
    }
    es.addEventListener('watch', (ev) => {
      const msg = parseWatchMessage((ev as MessageEvent<string>).data)
      if (msg) set({ ...snap, entries: applyWatchMessage(snap.entries, msg) })
    })
    es.onerror = () => {
      if (es.readyState !== EventSource.CLOSED) return
      es.close()
      source = null
      if (!opened) {
        set({ entries: new Map(), status: 'unavailable' })
        return
      }
      set({ ...snap, status: 'connecting' })
      timer = setTimer(() => {
        timer = null
        if (listeners.size > 0) connect()
      }, delay)
      delay = Math.min(delay * 2, BACKOFF_MAX_MS)
    }
  }

  function teardown() {
    source?.close()
    source = null
    if (timer) clearTimer(timer)
    timer = null
    delay = BACKOFF_START_MS
    snap = IDLE
  }

  return {
    subscribe(cb) {
      listeners.add(cb)
      if (listeners.size === 1) connect()
      return () => {
        listeners.delete(cb)
        if (listeners.size === 0) teardown()
      }
    },
    getSnapshot: () => snap,
  }
}

const sharedStore = createWatchStore(
  () => new EventSource('/api/v1/domains/watch'),
)

export function useWatchSnapshot(store: WatchStore = sharedStore) {
  return useSyncExternalStore(store.subscribe, store.getSnapshot)
}

// The entry for one hostname, or undefined when it is not watched, live, or
// the server has no watcher.
export function useDomainWatch(domain: string): WatchEntry | undefined {
  return useWatchSnapshot().entries.get(domain.toLowerCase())
}

// Mount once on the Domains page: a watched domain that turns live (or
// leaves the watch list) refreshes the domains list without a reload.
export function useDomainsWatchRefresh(store: WatchStore = sharedStore) {
  const { entries } = useWatchSnapshot(store)
  const queryClient = useQueryClient()
  const prev = useRef<WatchMap>(new Map())
  useEffect(() => {
    let changed = false
    prev.current.forEach((old, key) => {
      const now = entries.get(key)
      if (old.state !== 'live' && (!now || now.state === 'live')) changed = true
    })
    entries.forEach((now, key) => {
      const old = prev.current.get(key)
      if (now.state === 'live' && old && old.state !== 'live') changed = true
    })
    prev.current = entries
    if (changed)
      void queryClient.invalidateQueries({ queryKey: domainKeys.all })
  }, [entries, queryClient])
}
