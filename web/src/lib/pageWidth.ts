import { useSyncExternalStore } from 'react'

export type PageWidth = 'comfortable' | 'wide' | 'full'

export const PAGE_WIDTHS: PageWidth[] = ['comfortable', 'wide', 'full']

const STORAGE_KEY = 'ui-page-width'
const listeners = new Set<() => void>()

function isPageWidth(v: string | null): v is PageWidth {
  return v === 'comfortable' || v === 'wide' || v === 'full'
}

function read(): PageWidth {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    return isPageWidth(v) ? v : 'comfortable'
  } catch {
    return 'comfortable'
  }
}

let current: PageWidth = read()

function apply(value: PageWidth) {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-width', value)
  }
}

apply(current)

export function getPageWidth(): PageWidth {
  return current
}

export function setPageWidth(value: PageWidth) {
  current = value
  try {
    localStorage.setItem(STORAGE_KEY, value)
  } catch {
    // Storage can be blocked; the in-memory choice still applies this session.
  }
  apply(value)
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function usePageWidth(): PageWidth {
  return useSyncExternalStore(subscribe, getPageWidth, getPageWidth)
}
