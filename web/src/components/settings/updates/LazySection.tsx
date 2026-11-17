import { useState, type ReactNode } from 'react'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'

export interface LazySectionProps {
  title: string
  id?: string
  defaultOpen?: boolean
  children: ReactNode
}

// Children mount only once opened, so collapsed sections cost no requests.
export function LazySection({
  title,
  id,
  defaultOpen = false,
  children,
}: LazySectionProps) {
  const [open, setOpen] = useState(
    () => defaultOpen || (!!id && window.location.hash === `#${id}`),
  )
  return (
    <details
      id={id}
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
      className="group scroll-mt-20"
    >
      <summary className="flex cursor-pointer list-none items-center gap-2 rounded-md px-3 py-2.5 text-sm font-medium outline-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
        <ChevronRightGlassIcon
          aria-hidden="true"
          className="size-4 text-muted-foreground transition-transform group-open:rotate-90 motion-reduce:transition-none"
        />
        {title}
      </summary>
      {open ? <div className="space-y-3 px-3 pb-3">{children}</div> : null}
    </details>
  )
}
