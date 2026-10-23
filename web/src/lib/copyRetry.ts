import { useSyncExternalStore } from 'react'

let current: string | null = null
const listeners = new Set<() => void>()

function emit(): void {
  listeners.forEach((l) => {
    l()
  })
}

/** openCopyRetry asks the dashboard to show the retry dialog for one database. */
export function openCopyRetry(database: string): void {
  current = database
  emit()
}

export function closeCopyRetry(): void {
  current = null
  emit()
}

export function useCopyRetry(): string | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    () => current,
    () => null,
  )
}
