import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'

export interface BreadcrumbItem {
  key: string
  label: string
  /** Rendered by the caller so routing stays out of this primitive. */
  link?: ReactNode
}

// Presentational trail; the last item is the current page.
export function Breadcrumb({
  items,
  className,
}: {
  items: BreadcrumbItem[]
  className?: string
}) {
  const { t } = useTranslation('uiShared')
  return (
    <nav aria-label={t('breadcrumb.label')} className={className}>
      <ol className="flex flex-wrap items-center gap-1 text-sm">
        {items.map((item, i) => {
          const last = i === items.length - 1
          return (
            <li key={item.key} className="flex items-center gap-1">
              {i > 0 ? (
                <ChevronRightGlassIcon
                  aria-hidden="true"
                  className="size-4 shrink-0 text-muted-foreground/50 rtl:rotate-180"
                />
              ) : null}
              {last ? (
                <span
                  aria-current="page"
                  className="font-medium text-foreground"
                >
                  {item.label}
                </span>
              ) : (
                <span className="text-muted-foreground hover:text-foreground">
                  {item.link ?? item.label}
                </span>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
