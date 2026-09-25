export const CHORD_TIMEOUT_MS = 1500

export const GO_TARGETS: Record<string, { to: string; label: string }> = {
  a: { to: '/apps', label: 'Apps' },
  n: { to: '/nodes', label: 'Nodes' },
  s: { to: '/status', label: 'Status' },
  d: { to: '/domains', label: 'Domains' },
  b: { to: '/backups', label: 'Backups' },
  l: { to: '/loadbalancers', label: 'Load balancers' },
  t: { to: '/settings', label: 'Settings' },
}

export interface ShortcutDoc {
  keys: string[]
  description: string
}

export const SHORTCUT_DOCS: ShortcutDoc[] = [
  { keys: ['Ctrl/Cmd', 'K'], description: 'Open the command palette' },
  { keys: ['?'], description: 'Show keyboard shortcuts' },
  { keys: ['/'], description: 'Focus the search field on this page' },
  { keys: ['Esc'], description: 'Close a dialog, or leave the search field' },
  ...Object.entries(GO_TARGETS).map(([key, t]) => ({
    keys: ['g', key],
    description: `Go to ${t.label}`,
  })),
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
  | { type: 'go'; to: string }
  | { type: 'help' }
  | { type: 'focus-search' }
  | null

export const INITIAL_CHORD: ChordState = { pending: false, startedAt: 0 }

export function stepChord(
  state: ChordState,
  input: KeyInput,
  now: number,
): { state: ChordState; action: ShortcutAction } {
  if (
    input.typing ||
    input.dialogOpen ||
    input.ctrl ||
    input.meta ||
    input.alt
  ) {
    return { state: INITIAL_CHORD, action: null }
  }
  const live = state.pending && now - state.startedAt <= CHORD_TIMEOUT_MS
  if (live) {
    const target = GO_TARGETS[input.key]
    if (target) {
      return { state: INITIAL_CHORD, action: { type: 'go', to: target.to } }
    }
  }
  if (input.key === 'g') {
    return { state: { pending: true, startedAt: now }, action: null }
  }
  if (input.key === '?') {
    return { state: INITIAL_CHORD, action: { type: 'help' } }
  }
  if (input.key === '/') {
    return { state: INITIAL_CHORD, action: { type: 'focus-search' } }
  }
  return { state: INITIAL_CHORD, action: null }
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
