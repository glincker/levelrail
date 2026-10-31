import { useEffect, useState, type ReactNode } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { cn } from '@/lib/utils'

const HIGHLIGHT_MS = 2000

// Anchor target for one platform card: scrolls into view and briefly rings
// when the URL hash names it. scroll-mt clears the sticky header.
export function PlatformCardSection({
  anchor,
  children,
}: {
  anchor: string
  children: ReactNode
}) {
  const hash = useRouterState({ select: (s) => s.location.hash })
  const targeted = hash.replace(/^#/, '') === anchor
  const [expired, setExpired] = useState(false)
  const lit = targeted && !expired

  useEffect(() => {
    if (!targeted) return
    const el = document.getElementById(anchor)
    const reduced = window.matchMedia(
      '(prefers-reduced-motion: reduce)',
    ).matches
    el?.scrollIntoView({
      block: 'start',
      behavior: reduced ? 'auto' : 'smooth',
    })
    const timer = window.setTimeout(() => {
      setExpired(true)
    }, HIGHLIGHT_MS)
    return () => {
      window.clearTimeout(timer)
      setExpired(false)
    }
  }, [targeted, anchor])

  return (
    <section
      id={anchor}
      className={cn(
        'scroll-mt-20 rounded-lg transition-shadow motion-reduce:transition-none',
        lit && 'ring-2 ring-ring ring-offset-2 ring-offset-background',
      )}
    >
      {children}
    </section>
  )
}
