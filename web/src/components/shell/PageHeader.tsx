import * as React from 'react'

// Title row for pages: breadcrumb above, title beside a right-aligned actions slot.
export function PageHeader({
  breadcrumb,
  title,
  status,
  actions,
}: {
  breadcrumb?: React.ReactNode
  title: React.ReactNode
  status?: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <header className="space-y-2">
      {breadcrumb ? <div>{breadcrumb}</div> : null}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <h1 className="truncate text-lg font-semibold text-foreground">
            {title}
          </h1>
          {status}
        </div>
        {actions ? (
          <div className="flex items-center gap-2">{actions}</div>
        ) : null}
      </div>
    </header>
  )
}
