import { translateShared } from './activeLocale'
export type DeploymentKeyAction =
  | { type: 'move'; delta: 1 | -1 }
  | { type: 'open' }
  | { type: 'close' }
  | { type: 'add-filter' }
  | { type: 'redeploy' }
  | { type: 'rollback' }
  | { type: 'cancel' }

export interface DeploymentKeyInput {
  key: string
  ctrl: boolean
  meta: boolean
  alt: boolean
  typing: boolean
  /** A modal other than the drawer (confirm or promote dialog, menu) is open. */
  blockingOverlay: boolean
  drawerOpen: boolean
}

const SIMPLE: Record<string, DeploymentKeyAction> = {
  j: { type: 'move', delta: 1 },
  ArrowDown: { type: 'move', delta: 1 },
  k: { type: 'move', delta: -1 },
  ArrowUp: { type: 'move', delta: -1 },
  Enter: { type: 'open' },
  f: { type: 'add-filter' },
  r: { type: 'redeploy' },
  b: { type: 'rollback' },
  c: { type: 'cancel' },
}

export function deploymentKeyAction(
  input: DeploymentKeyInput,
): DeploymentKeyAction | null {
  if (input.ctrl || input.meta || input.alt) return null
  if (input.blockingOverlay) return null
  if (input.key === 'Escape') {
    return input.drawerOpen && !input.typing ? { type: 'close' } : null
  }
  if (input.typing) return null
  return SIMPLE[input.key] ?? null
}

/** Moves the focused row, clamping at both ends; starts at the top when nothing is focused. */
export function nextFocusId(
  ids: string[],
  current: string,
  delta: 1 | -1,
): string {
  if (ids.length === 0) return ''
  const idx = ids.indexOf(current)
  if (idx === -1) return (delta === 1 ? ids[0] : ids[ids.length - 1]) ?? ''
  const next = Math.min(Math.max(idx + delta, 0), ids.length - 1)
  return ids[next] ?? ''
}

function shortcutLabel(id: string, defaultValue: string): string {
  return translateShared(
    `deploys:deployList.shortcutLabels.${id}`,
    defaultValue,
  )
}

export const DEPLOYMENT_SHORTCUTS: { keys: string[]; label: string }[] = [
  {
    keys: ['/'],
    get label() {
      return shortcutLabel('search', 'Search')
    },
  },
  {
    keys: ['f'],
    get label() {
      return shortcutLabel('addFilter', 'Add filter')
    },
  },
  {
    keys: ['j', 'k'],
    get label() {
      return shortcutLabel('nextPrev', 'Next or previous deployment')
    },
  },
  {
    keys: ['Enter'],
    get label() {
      return shortcutLabel('open', 'Open details')
    },
  },
  {
    keys: ['Esc'],
    get label() {
      return shortcutLabel('close', 'Close details')
    },
  },
  {
    keys: ['r'],
    get label() {
      return shortcutLabel('redeploy', 'Redeploy (asks first)')
    },
  },
  {
    keys: ['b'],
    get label() {
      return shortcutLabel('rollback', 'Roll back to it (asks first)')
    },
  },
  {
    keys: ['c'],
    get label() {
      return shortcutLabel('cancel', 'Cancel a running deploy (asks first)')
    },
  },
]
