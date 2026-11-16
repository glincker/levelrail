import type { ReactNode } from 'react'

/** One flat, divided block for the failure diagnosis inside the Summary section. */
export function RunDiagnosis({ children }: { children: ReactNode }) {
  return (
    <div className="divide-y divide-border [&>*]:py-3 [&>*:first-child]:pt-0 [&>*:last-child]:pb-0 empty:hidden">
      {children}
    </div>
  )
}
