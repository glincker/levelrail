const ACTIVE_SELECTOR =
  '[data-slot="sidebar-content"] [data-sidebar="menu-button"][data-active]'

// Delays cover the lazy-loaded scoped nav and a collapsed group's 200ms expand.
const RETRY_DELAYS_MS = [0, 120, 300, 700]

/** Scrolls the sidebar's own scroller so the active row is fully visible; no-op when it already is. */
export function scrollActiveNavItemIntoView(root: ParentNode = document): void {
  const el = root.querySelector<HTMLElement>(ACTIVE_SELECTOR)
  const scroller = el?.closest<HTMLElement>(
    '[data-sidebar-scroll], [data-slot="sidebar-content"]',
  )
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

/** Resets the sidebar scroller to the top, then reveals the active row only if it is hidden; for when the mobile sheet opens. */
export function resetSidebarScroll(root: ParentNode = document): void {
  const scroller = root.querySelector<HTMLElement>(
    '[data-slot="sidebar-content"]',
  )
  if (!scroller) return
  scroller.scrollTop = 0
  scrollActiveNavItemIntoView(root)
}

/** Runs resetSidebarScroll across the sheet's mount and focus moves; returns a cancel function. */
export function scheduleResetSidebarScroll(root: ParentNode = document) {
  const timers = RETRY_DELAYS_MS.map((ms) =>
    window.setTimeout(() => {
      resetSidebarScroll(root)
    }, ms),
  )
  return () => {
    timers.forEach((t) => {
      window.clearTimeout(t)
    })
  }
}
