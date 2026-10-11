import type { ComponentType, ReactNode } from 'react'
import { Link as RouterLink, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CaretLeftIcon, CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import {
  parentCrumb,
  readListSearch,
  trafficCrumbs,
  type Crumb,
} from '@/lib/trafficBreadcrumbs'
import { CRUMB_CURRENT_CLASS, CRUMB_LINK_CLASS } from '@/components/Breadcrumbs'

// Crumb targets are data built by the model, not literals the route tree can
// type-check, so the typed Link is narrowed to a plain string-path link.
const Link = RouterLink as unknown as ComponentType<{
  to: string
  params?: Record<string, string>
  search?: Record<string, string>
  className?: string
  children?: ReactNode
}>

function useCrumbs(): Crumb[] | null {
  const { pathname, from } = useRouterState({
    select: (s) => ({
      pathname: s.location.pathname,
      from: (s.location.search as { from?: unknown }).from,
    }),
  })
  return trafficCrumbs({
    pathname,
    from: typeof from === 'string' ? from : undefined,
    listSearch: {
      domains: readListSearch('domains'),
      dns: readListSearch('dns'),
    },
  })
}

/** Domains / domain / tab and DNS / zone trails, a single back link on phones. */
export function TrafficBreadcrumb() {
  const { t } = useTranslation('traffic')
  const crumbs = useCrumbs()
  if (!crumbs || crumbs.length < 2) return null

  const text = (c: Crumb) =>
    'key' in c.label ? t(c.label.key as 'nav.domains') : c.label.text
  const parent = parentCrumb(crumbs)

  return (
    <>
      {parent ? (
        <Link
          to={parent.to ?? '/'}
          params={parent.params}
          search={parent.search}
          className={`${CRUMB_LINK_CLASS} flex items-center gap-1 text-xs sm:hidden`}
        >
          <CaretLeftIcon className="size-3.5" aria-hidden="true" />
          {text(parent)}
        </Link>
      ) : null}
      <nav
        aria-label="Breadcrumb"
        className="hidden flex-wrap items-center gap-1.5 text-xs sm:flex"
      >
        {crumbs.map((crumb, index) => (
          <span key={crumb.id} className="flex items-center gap-1.5">
            {index > 0 ? (
              <CaretRightIcon
                className="size-3 shrink-0 text-muted-foreground/50"
                aria-hidden="true"
              />
            ) : null}
            {crumb.to ? (
              <Link
                to={crumb.to}
                params={crumb.params}
                search={crumb.search}
                className={CRUMB_LINK_CLASS}
              >
                {text(crumb)}
              </Link>
            ) : (
              <span
                className={CRUMB_CURRENT_CLASS}
                aria-current={crumb.current ? 'page' : undefined}
              >
                {text(crumb)}
              </span>
            )}
          </span>
        ))}
      </nav>
    </>
  )
}
