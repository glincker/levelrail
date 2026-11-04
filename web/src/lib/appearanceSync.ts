import {
  APPEARANCE_DEFAULTS,
  getAppearance,
  parseAppearance,
  setAppearance,
  subscribeAppearance,
  type Appearance,
} from './appearance'
import {
  getDensity,
  setDensity,
  subscribeDensity,
  type Density,
} from './density'
import {
  getPageWidth,
  PAGE_WIDTHS,
  setPageWidth,
  subscribePageWidth,
  type PageWidth,
} from './pageWidth'

// Single place to tune how long local edits settle before they are sent.
export const APPEARANCE_SYNC_DEBOUNCE_MS =
  Number(import.meta.env.VITE_APPEARANCE_SYNC_DEBOUNCE_MS) || 800

export const SYNC_VERSION = 1
export const SYNC_ENABLED_KEY = 'levelrail-appearance-sync'
const MAX_IMPORT_BYTES = 64 * 1024

export interface SyncedAppearance {
  appearance: Appearance
  width: PageWidth
  density: Density
  /** Keys this client does not know; sent back untouched so a newer client keeps them. */
  extras: Record<string, unknown>
}

const KNOWN_KEYS = new Set<string>([
  'v',
  ...Object.keys(APPEARANCE_DEFAULTS),
  'width',
  'density',
])

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

/** parseSynced clamps a server or file value through parseAppearance; null when it is not an object. */
export function parseSynced(raw: unknown): SyncedAppearance | null {
  if (!isRecord(raw)) return null
  const extras: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(raw)) {
    if (!KNOWN_KEYS.has(k)) extras[k] = v
  }
  const width = PAGE_WIDTHS.find((w) => w === raw.width) ?? 'comfortable'
  const density: Density =
    raw.density === 'comfortable' ? 'comfortable' : 'compact'
  return {
    appearance: parseAppearance(JSON.stringify(raw)),
    width,
    density,
    extras,
  }
}

/** parseImport accepts only a JSON object that carries at least one known setting. */
export function parseImport(text: string): SyncedAppearance | null {
  if (text.length > MAX_IMPORT_BYTES) return null
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    return null
  }
  if (!isRecord(data)) return null
  const known = Object.keys(data).some((k) => k !== 'v' && KNOWN_KEYS.has(k))
  return known ? parseSynced(data) : null
}

export function buildPayload(
  extras: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    ...extras,
    v: SYNC_VERSION,
    ...getAppearance(),
    width: getPageWidth(),
    density: getDensity(),
  }
}

export function exportAppearanceJSON(): string {
  return JSON.stringify(buildPayload(), null, 2)
}

function isDefaultPayload(payload: Record<string, unknown>): boolean {
  const base = {
    v: SYNC_VERSION,
    ...APPEARANCE_DEFAULTS,
    width: 'comfortable',
    density: 'compact',
  }
  return JSON.stringify(sorted(payload)) === JSON.stringify(sorted(base))
}

function sorted(o: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(o).sort(([a], [b]) => (a < b ? -1 : 1)),
  )
}

function applySynced(value: SyncedAppearance) {
  setAppearance(value.appearance)
  setPageWidth(value.width)
  setDensity(value.density)
}

export type SyncStatus = 'idle' | 'syncing' | 'synced' | 'failed' | 'tooLarge'

export interface SyncState {
  status: SyncStatus
  syncedAt: number | null
  enabled: boolean
  /** unknown until the first read; no hides the row (route missing or token without a user). */
  available: 'unknown' | 'yes' | 'no'
}

export interface SyncApi {
  put(body: Record<string, unknown>): Promise<void>
  del(): Promise<void>
}

function readEnabled(): boolean {
  try {
    return localStorage.getItem(SYNC_ENABLED_KEY) !== 'off'
  } catch {
    return true
  }
}

function statusOf(err: unknown): number {
  const s = (err as { status?: unknown } | null)?.status
  return typeof s === 'number' ? s : 0
}

export function createAppearanceSync(
  api: SyncApi,
  opts: { debounceMs?: number; now?: () => number } = {},
) {
  const debounceMs = opts.debounceMs ?? APPEARANCE_SYNC_DEBOUNCE_MS
  const now = opts.now ?? Date.now
  const listeners = new Set<() => void>()
  let state: SyncState = {
    status: 'idle',
    syncedAt: null,
    enabled: readEnabled(),
    available: 'unknown',
  }
  let extras: Record<string, unknown> = {}
  let lastSent = ''
  let reconciled = false
  let suppress = false
  let timer: ReturnType<typeof setTimeout> | undefined

  function set(patch: Partial<SyncState>) {
    state = { ...state, ...patch }
    listeners.forEach((l) => l())
  }

  function cancel() {
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
  }

  async function flush() {
    cancel()
    if (!state.enabled || state.available === 'no') return
    const payload = buildPayload(extras)
    const json = JSON.stringify(sorted(payload))
    if (json === lastSent) return
    // Claimed before the await so an overlapping flush cannot send the same body twice.
    const previous = lastSent
    lastSent = json
    set({ status: 'syncing' })
    try {
      await api.put(payload)
      set({ status: 'synced', syncedAt: now() })
    } catch (err) {
      lastSent = previous
      const code = statusOf(err)
      if (code === 413) set({ status: 'tooLarge' })
      else if (code === 404 || code === 403)
        set({ available: 'no', status: 'idle' })
      else set({ status: 'failed' })
    }
  }

  function onLocalChange() {
    if (suppress || !reconciled || !state.enabled) return
    cancel()
    timer = setTimeout(() => void flush(), debounceMs)
  }

  return {
    getState: () => state,
    subscribe: (l: () => void) => {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    /** start watches the three local stores; returns the stop function. */
    start() {
      const offs = [
        subscribeAppearance(onLocalChange),
        subscribePageWidth(onLocalChange),
        subscribeDensity(onLocalChange),
      ]
      return () => {
        offs.forEach((off) => off())
        cancel()
      }
    },
    setAvailable(available: boolean) {
      set({ available: available ? 'yes' : 'no' })
    },
    setEnabled(enabled: boolean) {
      try {
        if (enabled) localStorage.removeItem(SYNC_ENABLED_KEY)
        else localStorage.setItem(SYNC_ENABLED_KEY, 'off')
      } catch {
        // Without storage the switch still holds for this session.
      }
      set({ enabled, status: 'idle' })
      if (!enabled) cancel()
      else if (reconciled) void flush()
    },
    /** reconcile applies the account-wins rule once the first read lands. */
    reconcile(server: unknown) {
      reconciled = true
      if (!state.enabled) return
      const parsed = parseSynced(server)
      if (parsed) {
        suppress = true
        try {
          applySynced(parsed)
        } finally {
          suppress = false
        }
        extras = parsed.extras
        lastSent = JSON.stringify(sorted(buildPayload(extras)))
        set({ status: 'synced', syncedAt: now() })
        return
      }
      if (!isDefaultPayload(buildPayload())) void flush()
    },
    /** importValue applies a validated file and pushes it right away. */
    importValue(value: SyncedAppearance) {
      applySynced(value)
      extras = value.extras
      if (state.enabled) void flush()
    },
    /** clearAccount drops the stored value after a local reset. */
    async clearAccount() {
      cancel()
      lastSent = ''
      if (!state.enabled || state.available === 'no') return
      set({ status: 'syncing' })
      try {
        await api.del()
        set({ status: 'synced', syncedAt: now() })
      } catch {
        set({ status: 'failed' })
      }
    },
    flush,
  }
}

export type AppearanceSync = ReturnType<typeof createAppearanceSync>

export type SyncLineKey =
  'off' | 'syncing' | 'tooLarge' | 'notSynced' | 'synced'

/** syncLineKey maps the engine state to the status line; pure so it is testable. */
export function syncLineKey(s: SyncState): SyncLineKey {
  if (!s.enabled) return 'off'
  if (s.status === 'syncing') return 'syncing'
  if (s.status === 'tooLarge') return 'tooLarge'
  if (s.status === 'synced' && s.syncedAt !== null) return 'synced'
  return 'notSynced'
}
