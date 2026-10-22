import { useEffect, useRef, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

const FOCUSABLE = 'button:not([disabled]), a[href]'

// Brings a deep-linked fix control into view and focuses it; it never clicks, the user still decides.
export function FixTarget({
  active,
  children,
}: {
  active: boolean
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [highlight, setHighlight] = useState(false)

  useEffect(() => {
    const el = ref.current
    if (!active || !el) return
    setHighlight(true)
    if (typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'center' })
    }
    el.querySelector<HTMLElement>(FOCUSABLE)?.focus()
  }, [active])

  return (
    <div
      ref={ref}
      data-highlight={highlight ? 'true' : undefined}
      className={cn(
        highlight && 'rounded-md ring-2 ring-primary ring-offset-2',
      )}
    >
      {children}
    </div>
  )
}
