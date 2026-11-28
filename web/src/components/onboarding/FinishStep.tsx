import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { TemplateLogoTile } from '../TemplateLogoTile'

const NEXT = [
  { key: 'deploy', to: '/apps/new' },
  { key: 'guide', to: '/get-started' },
] as const

// Catalog ids: a static site, a database and one popular app.
const STARTERS = ['nginx', 'postgres', 'uptime-kuma'] as const

/** FinishStep lists next places plus starter tiles that open the new-app page on a template. */
export function FinishStep() {
  const { t } = useTranslation('onboarding')
  return (
    <div className="space-y-3">
      <ul className="space-y-2">
        {NEXT.map((item) => (
          <li
            key={item.key}
            className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2.5"
          >
            <span className="min-w-0 text-sm">{t(`finish.${item.key}`)}</span>
            <Link
              to={item.to}
              className={buttonVariants({ size: 'sm', variant: 'outline' })}
            >
              {t('finish.open')}
              <ArrowRightGlassIcon aria-hidden="true" />
            </Link>
          </li>
        ))}
      </ul>
      <section aria-label={t('finish.starterLabel')} className="space-y-2">
        <h3 className="text-xs font-medium text-muted-foreground">
          {t('finish.starterLabel')}
        </h3>
        <ul className="grid grid-cols-1 gap-2 sm:grid-cols-3">
          {STARTERS.map((id) => (
            <li key={id}>
              <Link
                to="/apps/new"
                search={{ deploy: '1', template: id }}
                className="flex h-full items-center gap-2 rounded-lg border border-border px-2.5 py-2 text-sm font-medium hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none motion-reduce:transition-none"
              >
                <TemplateLogoTile id={id} name={t(`finish.starter.${id}`)} />
                <span className="min-w-0 truncate">
                  {t(`finish.starter.${id}`)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}
