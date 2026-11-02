import { useEffect, useRef, useState } from 'react'

const rawStall = Number(import.meta.env.VITE_UPGRADE_STALL_MS)

/** No progress change for this long counts as stuck; override with VITE_UPGRADE_STALL_MS. */
export const UPGRADE_STALL_MS =
  Number.isFinite(rawStall) && rawStall > 0 ? rawStall : 60_000

// Steps before the binary swap can still be abandoned safely.
const CANCELLABLE_STEPS = new Set([
  'guard',
  'download',
  'verify_checksum',
  'verify_signature',
  'probe_binary',
  'migration_check',
  'backup',
])

export interface StallInput {
  serverStalled?: boolean
  lastProgressAt?: string
  lastChangeAt: number
  now: number
  thresholdMs?: number
}

/** The server flag wins; otherwise the newest of server and client progress marks decides. */
export function isStalled(i: StallInput): boolean {
  if (i.serverStalled !== undefined) return i.serverStalled
  const server = i.lastProgressAt ? Date.parse(i.lastProgressAt) : 0
  const last = Math.max(Number.isNaN(server) ? 0 : server, i.lastChangeAt)
  return i.now - last >= (i.thresholdMs ?? UPGRADE_STALL_MS)
}

/** Percent 0 to 100 when the total is known, otherwise null (indeterminate). */
export function downloadPercent(
  downloaded?: number,
  total?: number,
): number | null {
  if (!total || total <= 0 || downloaded === undefined) return null
  return Math.min(100, Math.max(0, Math.round((downloaded / total) * 100)))
}

export type PrimaryAction = 'update' | 'cancel' | 'retry' | 'check'

export interface ActionInput {
  running: boolean
  stalled: boolean
  failed: boolean
  updateAvailable: boolean
  canApply: boolean
  step: string
  canCancel?: boolean
  canRetry?: boolean
  cancelRequested?: boolean
}

export interface ActionState {
  primary: PrimaryAction
  showCheck: boolean
  showUpdate: boolean
  showCancel: boolean
  showRetry: boolean
  cancelling: boolean
}

export function deriveActions(i: ActionInput): ActionState {
  const cancellable =
    i.canCancel ?? (i.step === '' || CANCELLABLE_STEPS.has(i.step))
  if (i.running) {
    // Retry on a stuck run means cancel first, then retry the failed attempt.
    const showCancel = cancellable
    const showRetry = i.stalled && cancellable
    const primary: PrimaryAction = showRetry
      ? 'retry'
      : showCancel
        ? 'cancel'
        : 'check'
    return {
      primary,
      showCheck: false,
      showUpdate: false,
      showCancel,
      showRetry,
      cancelling: i.cancelRequested === true,
    }
  }
  if (i.failed && i.canApply) {
    return {
      primary: 'retry',
      showCheck: true,
      showUpdate: false,
      showCancel: false,
      showRetry: i.canRetry ?? true,
      cancelling: false,
    }
  }
  const showUpdate = i.updateAvailable && i.canApply
  return {
    primary: showUpdate ? 'update' : 'check',
    showCheck: true,
    showUpdate,
    showCancel: false,
    showRetry: false,
    cancelling: false,
  }
}

/** Remembers when the progress signature last changed, for servers without last_progress_at. */
export function useLastChange(signature: string, active: boolean): number {
  const [at, setAt] = useState(() => Date.now())
  const prev = useRef(signature)
  useEffect(() => {
    if (!active) return
    if (prev.current !== signature) {
      prev.current = signature
      setAt(Date.now())
    }
  }, [signature, active])
  return at
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB']
  let v = n / 1024
  let u = 0
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  return `${v.toFixed(1)} ${units[u]}`
}
