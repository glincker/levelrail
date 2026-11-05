import { useSyncExternalStore } from 'react'

export type PageWidth = 'compact' | 'comfortable' | 'wide' | 'full' | 'custom'

export const PAGE_WIDTHS: PageWidth[] = [
  'compact',
  'comfortable',
  'wide',
  'full',
]
/** Every selectable width; custom reads its pixels from the appearance contentWidth. */
export const PAGE_WIDTH_CHOICES: PageWidth[] = [...PAGE_WIDTHS, 'custom']

// One source for the custom content width slider and its clamp.
export const CONTENT_WIDTH_MIN = 720
export const CONTENT_WIDTH_MAX = 1920
export const CONTENT_WIDTH_STEP = 40
export const CONTENT_WIDTH_DEFAULT = 1440

const STORAGE_KEY = 'ui-page-width'
const listeners = new Set<() => void>()

function isPageWidth(v: string | null): v is PageWidth {
  return PAGE_WIDTH_CHOICES.some((w) => w === v)
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

export { subscribe as subscribePageWidth }

export function usePageWidth(): PageWidth {
  return useSyncExternalStore(subscribe, getPageWidth, getPageWidth)
}
