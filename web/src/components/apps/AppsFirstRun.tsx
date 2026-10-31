import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  GitBranchGlassIcon,
  LayoutGridGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

const TILE =
  'group flex items-center gap-3 rounded-xl border border-border bg-card p-4 transition-colors hover:border-primary/50 focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50'

/** AppsFirstRun is the empty Apps page body: three one-click ways to get the first app running. */
export function AppsFirstRun() {
  const { t } = useTranslation('common')
  const paths = [
    {
      key: 'template',
      to: '/templates',
      Icon: LayoutGridGlassIcon,
      primary: true,
    },
    { key: 'git', to: '/apps/new', Icon: GitBranchGlassIcon, primary: false },
    { key: 'image', to: '/apps/new', Icon: PackageGlassIcon, primary: false },
  ] as const
  return (
    <section aria-labelledby="apps-first-run" className="space-y-3">
      <div>
        <h2 id="apps-first-run" className="text-base font-semibold">
          {t('appsList.firstRun.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('appsList.firstRun.subtitle')}
        </p>
      </div>
      <ul className="grid gap-3 md:grid-cols-3">
        {paths.map(({ key, to, Icon, primary }) => (
          <li key={key}>
            <Link
              to={to}
              className={cn(TILE, primary && 'border-primary/40 bg-primary/5')}
            >
              <span
                aria-hidden="true"
                className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-primary"
              >
                <Icon size="1.4em" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-medium text-foreground">
                  {t(`appsList.firstRun.${key}.title`)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(`appsList.firstRun.${key}.body`)}
                </span>
              </span>
              <ArrowRightGlassIcon
                aria-hidden="true"
                size="1.4em"
                className="shrink-0 text-muted-foreground motion-safe:transition-transform motion-safe:group-hover:translate-x-0.5"
              />
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
