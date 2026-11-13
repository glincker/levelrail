// DOM helpers for sliding bubbles around. They write a CSS variable per bubble instead of
// React state, so a pointer move costs no render. The variable feeds a translate class.
const VAR = '--bubble-x'

export function bubbleEls(row: Element): HTMLElement[] {
  return Array.from(row.querySelectorAll<HTMLElement>('[data-bubble]'))
}

export function isRtl(el: Element): boolean {
  return getComputedStyle(el).direction === 'rtl'
}

/** Distance between neighbouring bubble origins, gap included. */
export function slotWidth(els: readonly HTMLElement[]): number {
  const [a, b] = els
  if (!a) return 0
  const ra = a.getBoundingClientRect()
  if (!b) return ra.width
  return Math.abs(b.getBoundingClientRect().left - ra.left) || ra.width
}

export function setShift(el: HTMLElement, px: number): void {
  el.style.setProperty(VAR, `${px}px`)
}

export function clearShifts(row: Element): void {
  for (const el of bubbleEls(row)) {
    el.style.removeProperty(VAR)
    el.removeAttribute('data-dragging')
  }
}

/** Logical slot (0 = first in reading order) a pointer at clientX is closest to. */
export function insertIndexAt(
  els: readonly HTMLElement[],
  clientX: number,
  rtl: boolean,
): number {
  let before = 0
  for (const el of els) {
    const r = el.getBoundingClientRect()
    const centre = r.left + r.width / 2
    if (rtl ? clientX < centre : clientX > centre) before += 1
  }
  return before
}

/** Open a gap at logical index `at` by sliding later bubbles one slot along the reading direction. */
export function openGap(
  els: readonly HTMLElement[],
  at: number | null,
  slot: number,
  rtl: boolean,
): void {
  els.forEach((el, i) =>
    setShift(el, at !== null && i >= at ? slot * (rtl ? -1 : 1) : 0),
  )
}
