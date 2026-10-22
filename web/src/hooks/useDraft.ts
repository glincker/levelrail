import { useCallback, useEffect, useRef, useState } from 'react'
import { getStoredUsername } from '../lib/authStore'

export const DRAFT_SAVE_DEBOUNCE_MS = 500
const DEFAULT_TTL_HOURS = 72
const HOUR_MS = 3_600_000
const SECRET_FIELD = /password|secret|token|key/i
const STORAGE_PREFIX = 'draft'

export interface DraftOptions {
  /** Bump when the shape of the value changes; older drafts are dropped. */
  version?: number
  /** Extra field names never stored. Secret-looking names are always omitted. */
  omit?: readonly string[]
}

interface StoredDraft {
  v: number
  savedAt: number
  value: unknown
}

function ttlMs(): number {
  const hours = Number(
    import.meta.env.VITE_DRAFT_TTL_HOURS ?? DEFAULT_TTL_HOURS,
  )
  const valid = Number.isFinite(hours) && hours > 0 ? hours : DEFAULT_TTL_HOURS
  return valid * HOUR_MS
}

export function draftStorageKey(key: string): string {
  const user = getStoredUsername() ?? 'anon'
  const instance = typeof window === 'undefined' ? '' : window.location.host
  return `${STORAGE_PREFIX}:${instance}:${user}:${key}`
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function stripSecrets(value: unknown, omit: readonly string[]): unknown {
  if (Array.isArray(value)) return value.map((v) => stripSecrets(v, omit))
  if (!isRecord(value)) return value
  const names = new Set(omit.map((n) => n.toLowerCase()))
  const out: Record<string, unknown> = {}
  for (const [k, v] of Object.entries(value)) {
    if (SECRET_FIELD.test(k) || names.has(k.toLowerCase())) continue
    out[k] = stripSecrets(v, omit)
  }
  return out
}

function read(storageKey: string, version: number): StoredDraft | null {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!isRecord(parsed) || typeof parsed.savedAt !== 'number') {
      window.localStorage.removeItem(storageKey)
      return null
    }
    if (parsed.v !== version || Date.now() - parsed.savedAt > ttlMs()) {
      window.localStorage.removeItem(storageKey)
      return null
    }
    return { v: parsed.v, savedAt: parsed.savedAt, value: parsed.value }
  } catch {
    return null
  }
}

function write(storageKey: string, draft: StoredDraft): void {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(draft))
  } catch {
    // Storage blocked or full: the form keeps working, it just is not saved.
  }
}

function remove(storageKey: string): void {
  try {
    window.localStorage.removeItem(storageKey)
  } catch {
    // Nothing to clean up when storage is unavailable.
  }
}

export function useDraft<T extends object>(
  key: string,
  initial: T,
  options: DraftOptions = {},
) {
  const version = options.version ?? 1
  const omitRef = useRef<readonly string[]>([])
  const initialRef = useRef(initial)
  const storageKey = draftStorageKey(key)
  const [value, setValueState] = useState<T>(initial)
  const [stored, setStored] = useState<StoredDraft | null>(() =>
    read(storageKey, version),
  )
  const dirty = useRef(false)
  useEffect(() => {
    omitRef.current = options.omit ?? []
  })
  const pending = useRef<{ timer: number; value: T } | null>(null)

  const save = useCallback(
    (next: T) => {
      const safe = stripSecrets(next, omitRef.current)
      const blank = stripSecrets(initialRef.current, omitRef.current)
      if (JSON.stringify(safe) === JSON.stringify(blank)) {
        remove(storageKey)
        return
      }
      write(storageKey, { v: version, savedAt: Date.now(), value: safe })
    },
    [storageKey, version],
  )

  const cancelPending = useCallback(() => {
    if (pending.current) window.clearTimeout(pending.current.timer)
    pending.current = null
  }, [])

  const setValue = useCallback((next: T | ((prev: T) => T)) => {
    dirty.current = true
    setStored(null)
    setValueState(next)
  }, [])

  const update = useCallback(
    (patch: Partial<T>) => setValue((prev) => ({ ...prev, ...patch })),
    [setValue],
  )

  useEffect(() => {
    if (!dirty.current) return
    cancelPending()
    const timer = window.setTimeout(() => {
      pending.current = null
      save(value)
    }, DRAFT_SAVE_DEBOUNCE_MS)
    pending.current = { timer, value }
  }, [value, save, cancelPending])

  useEffect(
    () => () => {
      const p = pending.current
      if (!p) return
      window.clearTimeout(p.timer)
      save(p.value)
    },
    [save],
  )

  const restore = useCallback((): T => {
    const saved = stored && isRecord(stored.value) ? stored.value : {}
    const merged = { ...initialRef.current, ...saved } as T
    dirty.current = true
    setStored(null)
    setValueState(merged)
    return merged
  }, [stored])

  const discard = useCallback(() => {
    cancelPending()
    remove(storageKey)
    dirty.current = false
    setStored(null)
    setValueState(initialRef.current)
  }, [cancelPending, storageKey])

  const dismiss = useCallback(() => setStored(null), [])

  const clearOnSuccess = useCallback(() => {
    cancelPending()
    remove(storageKey)
    dirty.current = false
    setStored(null)
  }, [cancelPending, storageKey])

  return {
    value,
    setValue,
    update,
    hasDraft: stored !== null,
    savedAt: stored?.savedAt ?? null,
    restore,
    discard,
    dismiss,
    clearOnSuccess,
  }
}

export type FormDraft<T extends object> = ReturnType<typeof useDraft<T>>
