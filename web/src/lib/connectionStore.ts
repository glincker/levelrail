import { onlineManager, type QueryClient } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import {
  backoffDelayMs,
  isConnectivityError,
  recordFailure,
  shouldGoOffline,
} from './connectionState'

export interface ConnectionSnapshot {
  offline: boolean
  dismissed: boolean
  attempt: number
  probing: boolean
}

const INITIAL: ConnectionSnapshot = {
  offline: false,
  dismissed: false,
  attempt: 0,
  probing: false,
}

let snapshot: ConnectionSnapshot = INITIAL
let failures: number[] = []
let timer: ReturnType<typeof setTimeout> | undefined
let client: QueryClient | undefined
const listeners = new Set<() => void>()

function set(next: Partial<ConnectionSnapshot>): void {
  snapshot = { ...snapshot, ...next }
  listeners.forEach((l) => {
    l()
  })
}

export function subscribeConnection(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function getConnectionSnapshot(): ConnectionSnapshot {
  return snapshot
}

export function attachConnectionClient(queryClient: QueryClient): void {
  client = queryClient
}

async function probe(): Promise<boolean> {
  try {
    const res = await fetch('/healthz', { cache: 'no-store' })
    return res.ok
  } catch {
    return false
  }
}

function schedule(): void {
  clearTimeout(timer)
  timer = setTimeout(() => {
    void retryNow()
  }, backoffDelayMs(snapshot.attempt))
}

function reconnect(): void {
  clearTimeout(timer)
  failures = []
  set(INITIAL)
  onlineManager.setOnline(true)
  toast.add({ title: 'Reconnected', type: 'success' })
  void client?.invalidateQueries()
}

/** Probes /healthz now; reconnects on success, otherwise backs off further. */
export async function retryNow(): Promise<void> {
  if (!snapshot.offline || snapshot.probing) {
    return
  }
  clearTimeout(timer)
  set({ probing: true })
  if (await probe()) {
    reconnect()
    return
  }
  set({ probing: false, attempt: snapshot.attempt + 1 })
  schedule()
}

/** Feeds every query and mutation failure in; goes offline after repeated connectivity errors. */
export function reportError(error: unknown, now = Date.now()): void {
  if (!isConnectivityError(error) || snapshot.offline) {
    return
  }
  failures = recordFailure(failures, now)
  if (shouldGoOffline(failures)) {
    // Pausing the online manager also suspends every refetchInterval poll.
    onlineManager.setOnline(false)
    set({ offline: true, dismissed: false, attempt: 0, probing: false })
    schedule()
  }
}

export function reportSuccess(): void {
  failures = []
}

export function dismissConnectionBanner(): void {
  set({ dismissed: true })
}

export function resetConnectionForTests(): void {
  clearTimeout(timer)
  failures = []
  snapshot = INITIAL
  client = undefined
  listeners.clear()
  onlineManager.setOnline(true)
}
