import { useSyncExternalStore } from 'react'

const SCOPE_STORE_NAME = 'environment.scope.v1'
const SEARCH_PARAM = 'environment'

let active = false
let current = ''
const listeners = new Set<() => void>()

function readStored(): string {
  try {
    return window.localStorage.getItem(SCOPE_STORE_NAME) ?? ''
  } catch {
    return ''
  }
}

function writeStored(id: string): void {
  try {
    if (id === '') {
      window.localStorage.removeItem(SCOPE_STORE_NAME)
    } else {
      window.localStorage.setItem(SCOPE_STORE_NAME, id)
    }
  } catch {
    // Storage can be blocked; the in-memory scope still works.
  }
}

function mirrorToUrl(id: string): void {
  try {
    const url = new URL(window.location.href)
    if (id === '') {
      url.searchParams.delete(SEARCH_PARAM)
    } else {
      url.searchParams.set(SEARCH_PARAM, id)
    }
    window.history.replaceState(window.history.state, '', url)
  } catch {
    // A non-browser or locked-down history API just skips the mirror.
  }
}

function emit(): void {
  listeners.forEach((l) => {
    l()
  })
}

// Turns the scope on or off with the global-environments feature: when off
// every query behaves as before, whatever a past session stored.
export function activateEnvironmentScope(enabled: boolean): void {
  if (enabled === active) {
    return
  }
  active = enabled
  if (!enabled) {
    current = ''
  } else {
    const fromUrl = new URLSearchParams(window.location.search).get(
      SEARCH_PARAM,
    )
    current = fromUrl ?? readStored()
    if (fromUrl) {
      writeStored(fromUrl)
    }
  }
  emit()
}

// The selected environment id, or '' for all environments.
export function getEnvironmentScope(): string {
  return active ? current : ''
}

export function setEnvironmentScope(id: string): void {
  if (!active || id === current) {
    return
  }
  current = id
  writeStored(id)
  mirrorToUrl(id)
  emit()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

// Subscribes a component to scope changes; the value itself is read by the
// query option factories through getEnvironmentScope().
export function useEnvironmentScope(): string {
  return useSyncExternalStore(subscribe, getEnvironmentScope, () => '')
}

export function resetEnvironmentScopeForTests(): void {
  active = false
  current = ''
  listeners.clear()
}
