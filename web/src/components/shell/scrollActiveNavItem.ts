const ACTIVE_SELECTOR =
  '[data-slot="sidebar-content"] [data-sidebar="menu-button"][data-active]'

// Delays cover the lazy-loaded scoped nav and a collapsed group's 200ms expand.
const RETRY_DELAYS_MS = [0, 120, 300, 700]

/** Scrolls the sidebar's own scroller so the active row is fully visible; no-op when it already is. */
export function scrollActiveNavItemIntoView(root: ParentNode = document): void {
  const el = root.querySelector<HTMLElement>(ACTIVE_SELECTOR)
  const scroller = el?.closest<HTMLElement>('[data-slot="sidebar-content"]')
  if (!el || !scroller) return
  const box = scroller.getBoundingClientRect()
  const row = el.getBoundingClientRect()
  if (row.top >= box.top && row.bottom <= box.bottom) return
  const delta = row.top < box.top ? row.top - box.top : row.bottom - box.bottom
  const top = scroller.scrollTop + delta
  if (typeof scroller.scrollTo === 'function') {
    scroller.scrollTo({ top, behavior: 'instant' })
  } else {
    scroller.scrollTop = top
  }
}

/** Runs scrollActiveNavItemIntoView a few times and returns a cancel function. */
export function scheduleScrollActiveNavItem(root: ParentNode = document) {
  const timers = RETRY_DELAYS_MS.map((ms) =>
    window.setTimeout(() => {
      scrollActiveNavItemIntoView(root)
    }, ms),
  )
  return () => {
    timers.forEach((t) => {
      window.clearTimeout(t)
    })
  }
}
