import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '../shell/PageTemplate'

// Shared header for the platform pages: breadcrumb back to Platform,
// title, description, and a single column capped at 960px.
export function PlatformPageShell({
  title,
  description,
  icon,
  isOverview = false,
  children,
}: {
  title: string
  description: string
  icon: ReactNode
  isOverview?: boolean
  children: ReactNode
}) {
  const { t } = useTranslation('platformSettings')
  const crumb = 'text-muted-foreground hover:text-foreground hover:underline'
  return (
    <PageTemplate
      title={title}
      description={description}
      icon={icon}
      breadcrumb={
        <nav aria-label={t('breadcrumb.label')}>
          <ol className="flex flex-wrap items-center gap-1.5 text-xs">
            <li>
              <Link to="/settings" className={crumb}>
                {t('breadcrumb.settings')}
              </Link>
            </li>
            <li aria-hidden="true" className="text-muted-foreground">
              /
            </li>
            <li>
              {isOverview ? (
                <span aria-current="page" className="text-foreground">
                  {t('breadcrumb.platform')}
                </span>
              ) : (
                <Link to="/settings/platform" className={crumb}>
                  {t('breadcrumb.platform')}
                </Link>
              )}
            </li>
            {isOverview ? null : (
              <>
                <li aria-hidden="true" className="text-muted-foreground">
                  /
                </li>
                <li aria-current="page" className="text-foreground">
                  {title}
                </li>
              </>
            )}
          </ol>
        </nav>
      }
    >
      <div className="mx-auto w-full max-w-[60rem] space-y-4">{children}</div>
    </PageTemplate>
  )
}
