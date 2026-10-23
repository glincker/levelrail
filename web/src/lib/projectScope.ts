import { useSyncExternalStore } from 'react'

const STORE_NAME = 'project.scope.v1'
const SEARCH_PARAM = 'project'

const listeners = new Set<() => void>()

function read(): string {
  try {
    const fromUrl = new URLSearchParams(window.location.search).get(
      SEARCH_PARAM,
    )
    return fromUrl ?? window.localStorage.getItem(STORE_NAME) ?? ''
  } catch {
    return ''
  }
}

let current = read()

function persist(id: string): void {
  try {
    if (id === '') window.localStorage.removeItem(STORE_NAME)
    else window.localStorage.setItem(STORE_NAME, id)
    const url = new URL(window.location.href)
    if (id === '') url.searchParams.delete(SEARCH_PARAM)
    else url.searchParams.set(SEARCH_PARAM, id)
    window.history.replaceState(window.history.state, '', url)
  } catch {
    // Blocked storage or history just means the choice lasts for this visit.
  }
}

// The selected project id, or '' for all projects.
export function getProjectScope(): string {
  return current
}

export function setProjectScope(id: string): void {
  if (id === current) return
  current = id
  persist(id)
  listeners.forEach((l) => {
    l()
  })
}

export function useProjectScope(): string {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => {
        listeners.delete(l)
      }
    },
    getProjectScope,
    () => '',
  )
}

export function resetProjectScopeForTests(): void {
  current = ''
  listeners.clear()
}
