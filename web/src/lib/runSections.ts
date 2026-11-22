// Section ids, remembered open state and hash routing for the run page.

export const RUN_SECTIONS = [
  'logs',
  'waterfall',
  'summary',
  'rollout',
  'compare',
] as const

export type RunSectionId = (typeof RUN_SECTIONS)[number]

export function isRunSectionId(value: string): value is RunSectionId {
  return (RUN_SECTIONS as readonly string[]).includes(value)
}

const KEY_PREFIX = 'levelrail.run.section.'

export function readSectionOpen(id: RunSectionId, fallback: boolean): boolean {
  try {
    const raw = window.localStorage.getItem(KEY_PREFIX + id)
    if (raw === 'open') return true
    if (raw === 'closed') return false
  } catch {
    // Storage can be blocked; the page works without it.
  }
  return fallback
}

export function writeSectionOpen(id: RunSectionId, open: boolean): void {
  try {
    window.localStorage.setItem(KEY_PREFIX + id, open ? 'open' : 'closed')
  } catch {
    // Not remembering is fine.
  }
}

// #logs, #waterfall and a line permalink such as #L120 (which lives in Logs).
export function sectionFromHash(hash: string): RunSectionId | null {
  const value = hash.replace(/^#/, '')
  if (isRunSectionId(value)) return value
  return /^L\d+$/.test(value) ? 'logs' : null
}

export function sectionDomId(id: RunSectionId): string {
  return `run-section-${id}`
}

export const RUN_SECTION_OPEN_EVENT = 'levelrail:run-section-open'

export function scrollBehavior(): ScrollBehavior {
  const reduced =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  return reduced ? 'auto' : 'smooth'
}

// Asks the matching RunSection to open and flash, then scrolls to it.
export function goToSection(id: RunSectionId): void {
  window.dispatchEvent(
    new CustomEvent<RunSectionId>(RUN_SECTION_OPEN_EVENT, { detail: id }),
  )
  window.requestAnimationFrame(() => {
    document
      .getElementById(sectionDomId(id))
      ?.scrollIntoView({ behavior: scrollBehavior(), block: 'start' })
  })
  try {
    window.history.replaceState(null, '', `#${id}`)
  } catch {
    // Some embedders block history writes.
  }
}
