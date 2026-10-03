import { isFeatureVisible, type ExperimentalFeature } from './experimental'

export const CHORD_TIMEOUT_MS = 1500

// Hold threshold for the "l" stage-overlay gesture (useShortcuts.ts). Long
// enough that a normal keypress or the g-then-l chord never trips it.
export const LONG_PRESS_MS = 450

export const GO_TARGETS: Record<
  string,
  { to: string; label: string; feature?: ExperimentalFeature }
> = {
  a: { to: '/apps', label: 'Apps' },
  n: { to: '/nodes', label: 'Nodes' },
  s: { to: '/status', label: 'Status' },
  d: { to: '/domains', label: 'Domains' },
  b: { to: '/backups', label: 'Backups' },
  l: {
    to: '/loadbalancers',
    label: 'Load balancers',
    feature: 'load-balancer',
  },
  p: { to: '/pipelines', label: 'Pipelines' },
  m: { to: '/models', label: 'AI models', feature: 'ai-models' },
  t: { to: '/settings', label: 'Settings' },
}

export interface ShortcutDoc {
  keys: string[]
  description: string
}

// Shortcuts that aren't a "go to" tile, shown as a small reference strip
// inside StageOverlay.tsx alongside the GO_TARGETS tiles.
export const BASE_DOCS: ShortcutDoc[] = [
  { keys: ['Ctrl/Cmd', 'K'], description: 'Open the command palette' },
  { keys: ['Hold', 'L'], description: 'Open quick navigation' },
  { keys: ['/'], description: 'Focus the search field on this page' },
  { keys: ['Esc'], description: 'Close a dialog, or leave the search field' },
]

export interface ChordState {
  pending: boolean
  startedAt: number
}

export interface KeyInput {
  key: string
  ctrl: boolean
  meta: boolean
  alt: boolean
  typing: boolean
  dialogOpen: boolean
}

export type ShortcutAction =
  { type: 'go'; to: string } | { type: 'focus-search' } | null

export const INITIAL_CHORD: ChordState = { pending: false, startedAt: 0 }

// Shared by stepChord below and the long-press stage-overlay trigger
// (useShortcuts.ts): typing, an open dialog, or a modifier all suppress
// every keyboard shortcut the same way.
export function isShortcutInputSuppressed(input: KeyInput): boolean {
  return (
    input.typing || input.dialogOpen || input.ctrl || input.meta || input.alt
  )
}

export function stepChord(
  state: ChordState,
  input: KeyInput,
  now: number,
  enabled: readonly string[] = [],
): { state: ChordState; action: ShortcutAction } {
  if (isShortcutInputSuppressed(input)) {
    return { state: INITIAL_CHORD, action: null }
  }
  const live = state.pending && now - state.startedAt <= CHORD_TIMEOUT_MS
  if (live) {
    const target = GO_TARGETS[input.key]
    if (target && isFeatureVisible(target.feature, enabled)) {
      return { state: INITIAL_CHORD, action: { type: 'go', to: target.to } }
    }
  }
  if (input.key === 'g') {
    return { state: { pending: true, startedAt: now }, action: null }
  }
  if (input.key === '/') {
    return { state: INITIAL_CHORD, action: { type: 'focus-search' } }
  }
  return { state: INITIAL_CHORD, action: null }
}

// Base UI popups (dialog, menu, ...) keep their DOM node mounted with
// role="dialog"/role="menu" during the closing exit animation, and stay
// mounted indefinitely if that animation never resolves. `data-open` is
// only present while a popup is genuinely open, so appending it to a role
// selector is what tells "actually open" apart from "closed but mounted".
export const OPEN_OVERLAY_ATTR = '[data-open]'

export function isDialogOpen(root: ParentNode = document): boolean {
  return root.querySelector(`[role="dialog"]${OPEN_OVERLAY_ATTR}`) !== null
}

export function isTypingTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  if (el.isContentEditable) return true
  const tag = el.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'
}

const SEARCH_SELECTOR =
  'input[type="search"], input[data-shortcut-search], input[aria-label*="search" i], input[placeholder*="search" i], input[placeholder*="filter" i]'

export function findSearchField(
  root: ParentNode = document,
): HTMLElement | null {
  return root.querySelector<HTMLElement>(SEARCH_SELECTOR)
}

export function isSearchField(el: EventTarget | null): boolean {
  return el instanceof HTMLElement && el.matches(SEARCH_SELECTOR)
}
