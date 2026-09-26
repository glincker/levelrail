import { useSyncExternalStore, type ReactNode } from 'react'

export interface PageAction {
  key: string
  label: string
  icon: ReactNode
  run: () => void
  /** Shortcut hint shown in the command palette. */
  hint?: string[]
}

let current: PageAction[] = []
const listeners = new Set<() => void>()

/** Registers the active page's palette actions; returns the unregister fn. */
export function registerPageActions(actions: PageAction[]): () => void {
  current = actions
  listeners.forEach((l) => l())
  return () => {
    if (current === actions) {
      current = []
      listeners.forEach((l) => l())
    }
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function usePageActions(): PageAction[] {
  return useSyncExternalStore(
    subscribe,
    () => current,
    () => current,
  )
}
