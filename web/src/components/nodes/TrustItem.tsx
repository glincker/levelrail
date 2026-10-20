import type { ReactNode } from 'react'

// One label and value in a trust panel grid. Values wrap rather than
// truncate: a fingerprint cut short cannot be compared.
export function TrustItem({
  label,
  children,
  wide = false,
}: {
  label: string
  children: ReactNode
  wide?: boolean
}) {
  return (
    <div className={wide ? 'min-w-0 sm:col-span-2' : 'min-w-0'}>
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="mt-1 text-sm break-words text-foreground">{children}</dd>
    </div>
  )
}
