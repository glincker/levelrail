import { useSyncExternalStore } from 'react'

// The server reads these cookies to narrow every list, so the switcher
// filters requests the dashboard makes, SSE streams included.
const TEAM_COOKIE = 'scope_team'
const PROJECT_COOKIE = 'scope_project'

export interface TeamScope {
  team: string
  project: string
}

const listeners = new Set<() => void>()
let cached: TeamScope = { team: '', project: '' }
let cachedRaw = ''

function readCookie(name: string): string {
  try {
    for (const part of document.cookie.split(';')) {
      const [k, ...v] = part.trim().split('=')
      if (k === name) {
        return decodeURIComponent(v.join('='))
      }
    }
  } catch {
    // No document in tests or a locked-down browser: no scope.
  }
  return ''
}

function writeCookie(name: string, value: string): void {
  try {
    document.cookie =
      value === ''
        ? `${name}=; path=/; max-age=0; SameSite=Strict`
        : `${name}=${encodeURIComponent(value)}; path=/; SameSite=Strict`
  } catch {
    // Same as readCookie: the in-memory scope still updates listeners.
  }
}

export function getTeamScope(): TeamScope {
  const team = readCookie(TEAM_COOKIE)
  const project = readCookie(PROJECT_COOKIE)
  const raw = `${team}\u0000${project}`
  if (raw !== cachedRaw) {
    cachedRaw = raw
    cached = { team, project }
  }
  return cached
}

export function setTeamScope(next: TeamScope): void {
  writeCookie(TEAM_COOKIE, next.team)
  writeCookie(PROJECT_COOKIE, next.project)
  listeners.forEach((l) => {
    l()
  })
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

const EMPTY: TeamScope = { team: '', project: '' }

export function useTeamScope(): TeamScope {
  return useSyncExternalStore(subscribe, getTeamScope, () => EMPTY)
}
