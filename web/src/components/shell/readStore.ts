import * as React from 'react'
import { readJson, writeJson } from './persist'

export const READ_STORAGE_KEY = 'shell.notifications.read'

const isStringArray = (v: unknown): v is string[] =>
  Array.isArray(v) && v.every((x) => typeof x === 'string')

let current: readonly string[] | undefined
const listeners = new Set<() => void>()

function load(): readonly string[] {
  current ??= readJson(READ_STORAGE_KEY, [], isStringArray)
  return current
}

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function setReadIds(ids: string[]): void {
  current = ids
  writeJson(READ_STORAGE_KEY, ids)
  listeners.forEach((l) => {
    l()
  })
}

export function resetReadStoreForTests(): void {
  current = undefined
}

export function useReadIds(): {
  ids: readonly string[]
  set: ReadonlySet<string>
} {
  const ids = React.useSyncExternalStore(subscribe, load, load)
  const set = React.useMemo(() => new Set(ids), [ids])
  return { ids, set }
}
