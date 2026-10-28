const SKIP_RESTART_KEY = 'app-power:skip-restart-confirm'

/** Per-browser opt-out for the restart confirm; stop is never skippable. */
export function readSkipRestartConfirm(): boolean {
  try {
    return window.localStorage.getItem(SKIP_RESTART_KEY) === '1'
  } catch {
    return false
  }
}

export function writeSkipRestartConfirm(skip: boolean): void {
  try {
    if (skip) window.localStorage.setItem(SKIP_RESTART_KEY, '1')
    else window.localStorage.removeItem(SKIP_RESTART_KEY)
  } catch {
    // Storage can be blocked; the confirm just keeps showing.
  }
}

export type PowerKind = 'restart' | 'stop'
