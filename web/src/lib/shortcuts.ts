import { isFeatureVisible, type ExperimentalFeature } from './experimental'

export const CHORD_TIMEOUT_MS = 1500

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

const BASE_DOCS: ShortcutDoc[] = [
  { keys: ['Ctrl/Cmd', 'K'], description: 'Open the command palette' },
  { keys: ['?'], description: 'Show keyboard shortcuts' },
  { keys: ['/'], description: 'Focus the search field on this page' },
  { keys: ['Esc'], description: 'Close a dialog, or leave the search field' },
  { keys: ['Ctrl/Cmd', 'B'], description: 'Toggle the sidebar' },
]

// Shortcuts whose gated feature is off are left out.
export function shortcutDocsFor(enabled: readonly string[]): ShortcutDoc[] {
  return [
    ...BASE_DOCS,
    ...Object.entries(GO_TARGETS)
      .filter(([, t]) => isFeatureVisible(t.feature, enabled))
      .map(([key, t]) => ({
        keys: ['g', key],
        description: `Go to ${t.label}`,
      })),
  ]
}

export const SHORTCUT_DOCS: ShortcutDoc[] = shortcutDocsFor([
  'load-balancer',
  'ai-models',
])

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
  enabled: readonly string[] = [],
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
    if (target && isFeatureVisible(target.feature, enabled)) {
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
