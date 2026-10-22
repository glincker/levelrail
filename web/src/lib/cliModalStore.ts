import { useSyncExternalStore } from 'react'

// A tiny store so the shortcut, the palette entry and the header badge can
// all open the one modal without a provider in the route root.
let open = false
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((l) => {
    l()
  })
}

export function setCliModalOpen(next: boolean) {
  if (open !== next) {
    open = next
    emit()
  }
}

export function openCliModal() {
  setCliModalOpen(true)
}

export function useCliModalOpen(): boolean {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => {
        listeners.delete(cb)
      }
    },
    () => open,
    () => false,
  )
}
