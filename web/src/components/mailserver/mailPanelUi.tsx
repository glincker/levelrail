import type { ReactNode } from 'react'

/** Native select styled like Input; keeps the compact forms keyboard friendly. */
export const FIELD_CLASS =
  'h-8 rounded-md border border-border bg-background px-2 text-sm'

/** Wraps a control so a disabled state still explains itself on hover. */
export function Reason({
  why,
  children,
}: {
  why: string | undefined
  children: ReactNode
}) {
  return (
    <span title={why} className="inline-flex">
      {children}
    </span>
  )
}

export function PanelCard({
  label,
  title,
  children,
}: {
  label: string
  title: ReactNode
  children: ReactNode
}) {
  return (
    <section
      aria-label={label}
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 className="text-sm font-medium">{title}</h2>
      {children}
    </section>
  )
}
