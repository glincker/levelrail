export interface TocItem {
  id: string
  label: string
}

/** Top-most visible section wins; with none visible the last active one stays so the rail never blanks. */
export function pickActive(
  order: readonly string[],
  visible: ReadonlySet<string>,
  previous: string | null,
): string | null {
  for (const id of order) if (visible.has(id)) return id
  return previous
}

/** Watches the section elements and reports the active id; returns a disconnect function. */
export function createScrollSpy(
  ids: readonly string[],
  onChange: (id: string | null) => void,
  doc: Document = document,
  rootMargin = '-72px 0px -60% 0px',
): () => void {
  const visible = new Set<string>()
  let current: string | null = null
  const observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        const id = (e.target as HTMLElement).id
        if (e.isIntersecting) visible.add(id)
        else visible.delete(id)
      }
      const next = pickActive(ids, visible, current)
      if (next !== current) {
        current = next
        onChange(next)
      }
    },
    { rootMargin, threshold: 0 },
  )
  for (const id of ids) {
    const el = doc.getElementById(id)
    if (el) observer.observe(el)
  }
  return () => observer.disconnect()
}
