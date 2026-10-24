import type { ReactNode } from 'react'

/** One heading style for every New app section, with an optional trailing action. */
export function SectionHeading({
  id,
  children,
  action,
}: {
  id: string
  children: ReactNode
  action?: ReactNode
}) {
  return (
    <div className="flex min-h-7 flex-wrap items-center justify-between gap-2">
      <h2 id={id} className="text-base font-medium text-foreground">
        {children}
      </h2>
      {action}
    </div>
  )
}
