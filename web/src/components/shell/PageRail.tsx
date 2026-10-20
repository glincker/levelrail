import { useId, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** PageRail is the flat content column for PageTemplate's aside slot: section blocks, no cards. */
export function PageRail({
  className,
  children,
}: {
  className?: string
  children: ReactNode
}) {
  return (
    <div
      data-slot="page-rail"
      className={cn(
        'flex min-w-0 flex-col gap-6 border-t border-border pt-5 @min-[80rem]/page:border-t-0 @min-[80rem]/page:border-l @min-[80rem]/page:pt-0 @min-[80rem]/page:pl-5',
        className,
      )}
    >
      {children}
    </div>
  )
}

/** PageRailSection is a mono eyebrow heading followed by label and value rows. */
export function PageRailSection({
  title,
  children,
}: {
  title: ReactNode
  children: ReactNode
}) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="space-y-3">
      <h2
        id={id}
        className="font-(family-name:--glinr-font-eyebrow) text-(length:--glinr-type-size-eyebrow) leading-4 tracking-widest text-muted-foreground uppercase"
      >
        {title}
      </h2>
      <div className="space-y-3">{children}</div>
    </section>
  )
}

/** PageRailRow is a caption label over a body value. */
export function PageRailRow({
  label,
  children,
}: {
  label: ReactNode
  children: ReactNode
}) {
  return (
    <div className="min-w-0 space-y-0.5">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="min-w-0 text-sm break-words text-foreground">
        {children}
      </div>
    </div>
  )
}

/** PageRailList is a plain bulleted list of short body lines. */
export function PageRailList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-1 pl-4 text-sm text-foreground marker:text-muted-foreground">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  )
}
