// One answer for "should motion be reduced": the user's override on <html>
// (data-motion) wins, otherwise the system preference decides.
const QUERY = '(prefers-reduced-motion: reduce)'

export function prefersReducedMotion(): boolean {
  if (typeof document !== 'undefined') {
    const override = document.documentElement.getAttribute('data-motion')
    if (override === 'on') return true
    if (override === 'off') return false
  }
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia(QUERY).matches
  )
}
