import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  CircleCheckGlassIcon,
  CircleGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import { OverviewSuggestions } from './OverviewSuggestions'
import { computeSetup, type SuggestionDescriptor } from './suggestions'

const TARGET = {
  domain: '/apps/$name/domains',
  health: '/apps/$name/health',
  git: '/apps/$name/source',
  limits: '/apps/$name/resources',
} as const

/** Collapsed strip for setup extras so they never lead the page. */
export function RecommendedSetup({
  app,
  hasGitSource,
  imageDeployed,
  descriptors,
  dismissSuggestion,
  latest,
  onHide,
}: {
  app: AppDetail
  hasGitSource: boolean
  imageDeployed: boolean
  descriptors: SuggestionDescriptor[]
  dismissSuggestion: (id: string) => void
  latest?: DeployAttempt
  onHide: () => void
}) {
  const { t } = useTranslation('appAccess')
  const { items, done } = computeSetup({ app, hasGitSource, imageDeployed })
  return (
    <details className="group rounded-xl border border-border bg-card">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 text-sm">
        <span className="font-medium">{t('setup.title')}</span>
        <span className="flex items-center gap-2 text-muted-foreground">
          {t('setup.progress', { done, total: items.length })}
          <ChevronDownGlassIcon
            className="size-4.5 transition-transform group-open:rotate-180 motion-reduce:transition-none"
            aria-hidden="true"
          />
        </span>
      </summary>
      <div className="space-y-3 border-t border-border px-4 py-3">
        <ul className="grid gap-1 sm:grid-cols-2">
          {items.map((i) => (
            <li key={i.id}>
              {i.done ? (
                <span className="flex items-center gap-2 py-1 text-muted-foreground line-through">
                  <CircleCheckGlassIcon
                    className="size-4.5 text-tone-success"
                    aria-hidden="true"
                  />
                  {i.label}
                </span>
              ) : (
                <Link
                  to={TARGET[i.id]}
                  params={{ name: app.name }}
                  className="flex items-center gap-2 rounded-md py-1 hover:bg-muted"
                >
                  <CircleGlassIcon className="size-4.5" aria-hidden="true" />
                  {i.label}
                </Link>
              )}
            </li>
          ))}
        </ul>
        <OverviewSuggestions
          app={app}
          descriptors={descriptors}
          dismiss={dismissSuggestion}
          latest={latest}
        />
        <button
          type="button"
          onClick={onHide}
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          <XGlassIcon className="size-3.5" aria-hidden="true" />
          {t('setup.hide')}
        </button>
      </div>
    </details>
  )
}
