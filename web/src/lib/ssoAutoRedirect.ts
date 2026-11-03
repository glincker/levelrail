// Long enough to read the notice and press Cancel, short enough to feel automatic.
export const SSO_REDIRECT_DELAY_MS =
  Number(import.meta.env.VITE_SSO_REDIRECT_DELAY_MS) || 800
export const SSO_LOOP_GUARD_MS =
  Number(import.meta.env.VITE_SSO_LOOP_GUARD_MS) || 60_000
export const SSO_GUARD_KEY = 'sso-auto-redirect-at'

export function recentlyAttempted(): boolean {
  try {
    const at = Number(window.sessionStorage.getItem(SSO_GUARD_KEY))
    return at > 0 && Date.now() - at < SSO_LOOP_GUARD_MS
  } catch {
    return false
  }
}

export function markAttempt(): void {
  try {
    window.sessionStorage.setItem(SSO_GUARD_KEY, String(Date.now()))
  } catch {
    // Storage blocked: the other guards still apply.
  }
}
