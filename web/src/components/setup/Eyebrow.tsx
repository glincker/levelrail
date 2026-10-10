import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** Eyebrow is the mono uppercase label with a leading accent dot used above setup headings. */
export function Eyebrow({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <p
      className={cn(
        'flex items-center gap-2 font-mono text-xs uppercase tracking-widest text-muted-foreground',
        className,
      )}
    >
      <span
        className="size-1.5 rounded-full bg-tone-accent-solid"
        aria-hidden="true"
      />
      {children}
    </p>
  )
}
