import { Link, useRouter, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon } from '@thesvg/ui-react'
import { relatedLinksFor } from '@/lib/relatedLinks'
import { cn } from '@/lib/utils'

function RelatedLinksInner({ className }: { className?: string }) {
  const { t } = useTranslation('relatedLinks')
  const routeId = useRouterState({
    select: (s) => s.matches[s.matches.length - 1]?.routeId ?? '',
  })
  const entries = relatedLinksFor(routeId)
  if (entries.length === 0) return null
  return (
    <nav aria-label={t('label')} className={className}>
      <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
        <li className="text-xs font-medium text-muted-foreground">
          {t('heading')}
        </li>
        {entries.map((e) => (
          <li key={`${e.to}-${e.labelKey}`}>
            <Link
              to={e.to}
              params={e.inheritParams ? true : undefined}
              className={cn(
                'inline-flex items-center gap-1 rounded-sm text-primary underline-offset-4 outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring',
              )}
            >
              {t(`links.${e.labelKey}`)}
              <ArrowRightGlassIcon size="1.2em" aria-hidden="true" />
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  )
}

/** RelatedLinks renders the cross-links for the current route, or nothing when the map has none. */
export function RelatedLinks({ className }: { className?: string }) {
  // Pages rendered outside a router (tests, previews) simply have no links.
  const router = useRouter({ warn: false })
  if (!router) return null
  return <RelatedLinksInner className={className} />
}
