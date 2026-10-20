import { useSyncExternalStore } from 'react'

export type Density = 'compact' | 'comfortable'

const STORAGE_KEY = 'ui-density'
const listeners = new Set<() => void>()

function read(): Density {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'comfortable'
      ? 'comfortable'
      : 'compact'
  } catch {
    return 'compact'
  }
}

let current: Density = read()

function apply(value: Density) {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-density', value)
  }
}

apply(current)

export function getDensity(): Density {
  return current
}

export function setDensity(value: Density) {
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

export function useDensity(): Density {
  return useSyncExternalStore(subscribe, getDensity, getDensity)
}
