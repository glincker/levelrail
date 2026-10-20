import { useSyncExternalStore } from 'react'
import {
  BOTTOM_MIN,
  SIDE_MIN,
  type Corner,
  type DockMode,
  type DockView,
  type Rect,
} from './dockGeometry'

const STORAGE_KEY = 'dock.state.v2'
const MODES: readonly DockMode[] = ['float', 'bottom', 'side']
const VIEWS: readonly DockView[] = ['open', 'minimized', 'closed']
const CORNERS: readonly Corner[] = ['br', 'bl', 'tr', 'tl']

export interface DockState {
  view: DockView
  mode: DockMode
  maximized: boolean
  push: boolean
  tab: string
  bottom: number
  side: number
  corner: Corner
  floats: Record<string, Rect>
  unread: boolean
}

const DEFAULTS: DockState = {
  view: 'minimized',
  mode: 'float',
  maximized: false,
  push: false,
  tab: 'ai-chat',
  bottom: 360,
  side: 420,
  corner: 'br',
  floats: {},
  unread: false,
}

function isRect(value: unknown): value is Rect {
  if (typeof value !== 'object' || value === null) return false
  const r = value as Record<string, unknown>
  return ['x', 'y', 'w', 'h'].every(
    (k) => typeof r[k] === 'number' && Number.isFinite(r[k]),
  )
}

function oneOf<T extends string>(
  list: readonly T[],
  value: unknown,
  fallback: T,
): T {
  return list.find((x) => x === value) ?? fallback
}

function num(value: unknown, fallback: number, min: number): number {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.max(min, value)
    : fallback
}

function load(): DockState {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return DEFAULTS
    const p = JSON.parse(raw) as Record<string, unknown>
    const floats: Record<string, Rect> = {}
    if (typeof p.floats === 'object' && p.floats !== null) {
      for (const [k, v] of Object.entries(p.floats)) {
        if (isRect(v)) floats[k] = v
      }
    }
    return {
      view: oneOf(VIEWS, p.view, DEFAULTS.view),
      mode: oneOf(MODES, p.mode, DEFAULTS.mode),
      maximized: p.maximized === true,
      push: p.push === true,
      tab: typeof p.tab === 'string' ? p.tab : DEFAULTS.tab,
      bottom: num(p.bottom, DEFAULTS.bottom, BOTTOM_MIN),
      side: num(p.side, DEFAULTS.side, SIDE_MIN),
      corner: oneOf(CORNERS, p.corner, DEFAULTS.corner),
      floats,
      unread: false,
    }
  } catch {
    return DEFAULTS
  }
}

function save(s: DockState) {
  try {
    window.localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ ...s, unread: undefined }),
    )
  } catch {
    // storage unavailable: state stays in memory only
  }
}

let state: DockState = load()
const listeners = new Set<() => void>()

function commit(patch: Partial<DockState>, persist = true) {
  state = { ...state, ...patch }
  if (persist) save(state)
  listeners.forEach((l) => l())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function getDockState(): DockState {
  return state
}

export function useDockState(): DockState {
  return useSyncExternalStore(subscribe, () => state)
}

export function setDockView(view: DockView) {
  if (view === state.view) return
  commit({ view, unread: view === 'open' ? false : state.unread })
}

export function toggleDock() {
  setDockView(state.view === 'open' ? 'closed' : 'open')
}

export function setDockMode(mode: DockMode) {
  commit({ mode, maximized: false })
}

export function cycleDockMode() {
  const next = MODES[(MODES.indexOf(state.mode) + 1) % MODES.length] ?? 'float'
  setDockMode(next)
}

export function toggleDockMaximized() {
  commit({ maximized: !state.maximized })
}

export function setDockPush(push: boolean) {
  commit({ push })
}

export function setDockTab(tab: string) {
  commit({
    tab,
    unread: tab === 'ai-chat' && state.view === 'open' ? false : state.unread,
  })
}

export function setDockBottom(bottom: number) {
  commit({ bottom: Math.round(bottom), maximized: false })
}

export function setDockSide(side: number) {
  commit({ side: Math.round(side), maximized: false })
}

export function setDockFloat(sizeClass: string, rect: Rect) {
  commit({
    floats: {
      ...state.floats,
      [sizeClass]: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        w: Math.round(rect.w),
        h: Math.round(rect.h),
      },
    },
    maximized: false,
  })
}

export function setDockCorner(corner: Corner) {
  commit({ corner })
}

export function resetDockGeometry(sizeClass: string) {
  const floats = { ...state.floats }
  delete floats[sizeClass]
  commit({
    floats,
    bottom: DEFAULTS.bottom,
    side: DEFAULTS.side,
    corner: DEFAULTS.corner,
    maximized: false,
  })
}

// An assistant reply that lands while the chat is not in front.
export function noteDockReply() {
  if (state.view === 'open' && state.tab === 'ai-chat') return
  if (!state.unread) commit({ unread: true }, false)
}

export function clearDockUnread() {
  if (state.unread) commit({ unread: false }, false)
}
