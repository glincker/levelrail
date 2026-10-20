import { useCallback, useEffect, useState } from 'react'

function storageKey(app: string, domain: string, tab: string): string {
  return `domain-policy-draft:${app}:${domain}:${tab}`
}

function readDraft<T>(key: string): T | null {
  try {
    const raw = window.localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    return null
  }
}

function writeDraft(key: string, value: unknown): void {
  try {
    if (value === null) window.localStorage.removeItem(key)
    else window.localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage can be blocked (private mode); the draft then lives in memory only.
  }
}

// usePolicyDraft keeps an editable copy of saved, persisted per tab so a
// reload keeps unsaved work. Dirty is a structural comparison to saved.
export function usePolicyDraft<T>(
  app: string,
  domain: string,
  tab: string,
  saved: T,
) {
  const key = storageKey(app, domain, tab)
  const [draft, setDraftState] = useState<T>(() => readDraft<T>(key) ?? saved)
  const savedJSON = JSON.stringify(saved)
  const dirty = JSON.stringify(draft) !== savedJSON

  useEffect(() => {
    writeDraft(key, dirty ? draft : null)
  }, [key, draft, dirty])

  const setDraft = useCallback((next: T | ((prev: T) => T)) => {
    setDraftState(next)
  }, [])

  const discard = useCallback(() => {
    setDraftState(JSON.parse(savedJSON) as T)
    writeDraft(key, null)
  }, [key, savedJSON])

  return { draft, setDraft, dirty, discard }
}

export interface DiffLine {
  path: string
  before: string
  after: string
}

function flatten(
  value: unknown,
  prefix: string,
  out: Map<string, string>,
): void {
  if (value !== null && typeof value === 'object') {
    const entries = Array.isArray(value)
      ? value.map((v, i) => [String(i), v] as const)
      : Object.entries(value as Record<string, unknown>)
    if (entries.length === 0) out.set(prefix || '(root)', '(empty)')
    for (const [k, v] of entries) {
      flatten(v, prefix ? `${prefix}.${k}` : k, out)
    }
    return
  }
  if (value === undefined) return
  out.set(prefix || '(root)', JSON.stringify(value))
}

// diffFields lists every leaf that differs, as dotted paths.
export function diffFields(before: unknown, after: unknown): DiffLine[] {
  const a = new Map<string, string>()
  const b = new Map<string, string>()
  flatten(before, '', a)
  flatten(after, '', b)
  const keys = new Set([...a.keys(), ...b.keys()])
  const out: DiffLine[] = []
  for (const k of [...keys].sort()) {
    const x = a.get(k) ?? ''
    const y = b.get(k) ?? ''
    if (x !== y) out.push({ path: k, before: x, after: y })
  }
  return out
}

// useDebounced returns value after it stopped changing for delay ms.
export function useDebounced<T>(value: T, delay = 400): T {
  const [out, setOut] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setOut(value), delay)
    return () => window.clearTimeout(id)
  }, [value, delay])
  return out
}
