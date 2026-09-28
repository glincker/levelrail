import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import type { DocHeading } from '@/types/docs'

interface DocsOnThisPageProps {
  headings: DocHeading[]
  className?: string
}

// Tracks which heading is currently in view via IntersectionObserver and
// highlights it, VitePress/Mintlify-style: nested under the active page in
// the sidebar (see routes/help.tsx), so scrolling the doc visibly moves
// the highlight down the left nav instead of the sidebar staying static
// once you've landed on a page. The parent renders this with `key={routePath}`
// so navigating to a different doc remounts it (headings are DOM nodes
// from the just-rendered page, not stable across navigation) instead of
// needing an effect to reset state on prop change.
export function DocsOnThisPage({ headings, className }: DocsOnThisPageProps) {
  const [activeId, setActiveId] = useState<string | null>(
    () => headings[0]?.id ?? null,
  )
  const visibleIds = useRef<Set<string>>(new Set())

  useEffect(() => {
    if (headings.length === 0) return

    let cancelled = false
    let frame: number
    let cleanupObserver: (() => void) | undefined

    // The doc's own content (and so its heading elements) loads through
    // /help/$'s separate route loader, independently of the manifest
    // this component's headings prop comes from: on navigation, this
    // effect can easily run before those elements exist yet. Retry each
    // frame until at least one shows up, rather than silently observing
    // nothing and leaving the highlight stuck on the first heading.
    function trySetup() {
      const elements = headings
        .map((h) => document.getElementById(h.id))
        .filter((el): el is HTMLElement => el !== null)

      if (elements.length === 0) {
        frame = requestAnimationFrame(trySetup)
        return
      }
      if (cancelled) return

      const observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) {
              visibleIds.current.add(entry.target.id)
            } else {
              visibleIds.current.delete(entry.target.id)
            }
          }
          // The topmost currently-visible heading wins, matching reading
          // order rather than intersection-observer callback order.
          const firstVisible = headings.find((h) =>
            visibleIds.current.has(h.id),
          )
          if (firstVisible) setActiveId(firstVisible.id)
        },
        { rootMargin: '-80px 0px -70% 0px', threshold: 0 },
      )

      for (const el of elements) observer.observe(el)
      cleanupObserver = () => observer.disconnect()
    }

    trySetup()

    return () => {
      cancelled = true
      cancelAnimationFrame(frame)
      cleanupObserver?.()
    }
  }, [headings])

  if (headings.length === 0) return null

  return (
    <ul className={cn('space-y-0.5 border-l border-border', className)}>
      {headings.map((heading) => {
        const active = heading.id === activeId
        return (
          <li key={heading.id}>
            <a
              href={`#${heading.id}`}
              className={cn(
                'block -ml-px border-l px-3 py-1 text-xs transition-colors',
                heading.level === 3 && 'pl-6',
                active
                  ? 'border-foreground font-medium text-foreground'
                  : 'border-transparent text-muted-foreground hover:border-border hover:text-foreground',
              )}
            >
              {heading.text}
            </a>
          </li>
        )
      })}
    </ul>
  )
}
