// Focuses the first control marked aria-invalid inside a form and scrolls
// it into view, without animation when the user prefers reduced motion.
export function focusFirstInvalid(root: ParentNode | null): HTMLElement | null {
  const el = root?.querySelector<HTMLElement>('[aria-invalid="true"]') ?? null
  if (!el) return null
  focusAndReveal(el)
  return el
}

export function focusAndReveal(el: HTMLElement): void {
  el.focus({ preventScroll: true })
  const reduce =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (typeof el.scrollIntoView === 'function') {
    el.scrollIntoView({
      block: 'center',
      behavior: reduce ? 'auto' : 'smooth',
    })
  }
}
