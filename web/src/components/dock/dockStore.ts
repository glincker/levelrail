import { useSyncExternalStore } from 'react'

const OPEN_KEY = 'dock.open'
const HEIGHT_KEY = 'dock.height'
export const DOCK_MIN_HEIGHT = 200
export const DOCK_DEFAULT_HEIGHT = 360

interface DockState {
  open: boolean
  height: number
}

function read(key: string): string | null {
  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string) {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // storage unavailable: state stays in memory only
  }
}

export function clampDockHeight(height: number): number {
  const max =
    typeof window === 'undefined'
      ? 800
      : Math.max(DOCK_MIN_HEIGHT, Math.floor(window.innerHeight * 0.8))
  return Math.min(max, Math.max(DOCK_MIN_HEIGHT, Math.round(height)))
}

function load(): DockState {
  const stored = Number(read(HEIGHT_KEY))
  return {
    open: read(OPEN_KEY) === '1',
    height:
      Number.isFinite(stored) && stored > 0
        ? clampDockHeight(stored)
        : DOCK_DEFAULT_HEIGHT,
  }
}

let state: DockState = load()
const listeners = new Set<() => void>()

function commit(next: DockState) {
  state = next
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function setDockOpen(open: boolean) {
  if (open === state.open) return
  write(OPEN_KEY, open ? '1' : '0')
  commit({ ...state, open })
}

export function toggleDock() {
  setDockOpen(!state.open)
}

export function setDockHeight(height: number, persist = true) {
  const next = clampDockHeight(height)
  if (next === state.height) return
  if (persist) write(HEIGHT_KEY, String(next))
  commit({ ...state, height: next })
}

export function persistDockHeight() {
  write(HEIGHT_KEY, String(state.height))
}

export function useDockOpen(): boolean {
  return useSyncExternalStore(subscribe, () => state.open)
}

export function useDockHeight(): number {
  return useSyncExternalStore(subscribe, () => state.height)
}
