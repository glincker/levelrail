import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '../shell/PageTemplate'

// Shared header for the platform pages: breadcrumb, title, description, tabs
// in the page template's tabs slot, and a single column capped at 960px.
export function PlatformPageShell({
  title,
  description,
  icon,
  tabs,
  children,
}: {
  title: string
  description: string
  icon: ReactNode
  tabs: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation('platformSettings')
  return (
    <PageTemplate
      title={title}
      description={description}
      icon={icon}
      tabs={tabs}
      breadcrumb={
        <nav aria-label={t('breadcrumb.label')}>
          <ol className="flex flex-wrap items-center gap-1.5 text-xs">
            <li>
              <Link
                to="/settings"
                className="text-muted-foreground hover:text-foreground hover:underline"
              >
                {t('breadcrumb.settings')}
              </Link>
            </li>
            <li aria-hidden="true" className="text-muted-foreground">
              /
            </li>
            <li aria-current="page" className="text-foreground">
              {t('breadcrumb.platform')}
            </li>
          </ol>
        </nav>
      }
    >
      <div className="mx-auto w-full max-w-[60rem] space-y-4">{children}</div>
    </PageTemplate>
  )
}
