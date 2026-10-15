import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

// An empty state that shows what the page looks like once it has data: the
// sample content is real UI, faded out, inert and hidden from assistive tech,
// with the call to action floating over it.
export function GhostPreview({
  sample,
  children,
  className,
}: {
  sample: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'relative min-h-[28rem] overflow-hidden rounded-xl border border-border bg-card',
        className,
      )}
    >
      <div
        aria-hidden="true"
        inert
        className="pointer-events-none opacity-70 select-none [mask-image:linear-gradient(to_bottom,black_30%,transparent_100%)]"
      >
        {sample}
      </div>
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 bg-linear-to-b from-transparent via-background/40 to-background/80"
      />
      <div className="absolute inset-0 flex items-center justify-center px-6 py-10">
        <div className="w-full max-w-lg rounded-2xl border border-border bg-card px-6 py-8 text-center shadow-lg">
          {children}
        </div>
      </div>
    </div>
  )
}
