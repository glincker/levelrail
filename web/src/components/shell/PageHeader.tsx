import * as React from 'react'
import { HelpLink } from '../HelpLink'

// Title row for pages: breadcrumb above, title beside a right-aligned actions slot.
export function PageHeader({
  breadcrumb,
  title,
  description,
  status,
  helpPath,
  helpLabel,
  actions,
}: {
  breadcrumb?: React.ReactNode
  title: React.ReactNode
  /** Optional subtitle rendered below the title row. */
  description?: React.ReactNode
  status?: React.ReactNode
  /** Docs path for an inline help link next to the title. Requires `helpLabel`. */
  helpPath?: string
  helpLabel?: string
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
          {helpPath && helpLabel ? (
            <HelpLink path={helpPath} label={helpLabel} />
          ) : null}
        </div>
        {actions ? (
          <div className="flex items-center gap-2">{actions}</div>
        ) : null}
      </div>
      {description ? (
        <p className="text-sm text-muted-foreground">{description}</p>
      ) : null}
    </header>
  )
}
