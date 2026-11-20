import { lazy, Suspense, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APP_LIST_GRID } from '../AppRow'
import type { AppListEntry } from '../../types/appDetail'
import type { ViewMode } from '../../lib/appsListView'
import { certificatesQueryOptions } from '../../queries/certificates'
import { worstCertByApp } from '../../lib/certTruth'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { AppCardSkeleton, CARD_GRID_CLASS } from './AppCardSkeleton'
import { AppsTable } from './AppsTable'

export function ListHeader() {
  return (
    <div
      className={`${APP_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium text-muted-foreground`}
    >
      <span aria-hidden="true" />
      <span aria-hidden="true" />
      <span>Name</span>
      <span className="hidden lg:block">Traffic 1h</span>
      <span className="hidden lg:block">p95</span>
      <span className="hidden lg:block">Errors</span>
      <span className="hidden lg:block">Deployed</span>
      <span aria-hidden="true" />
    </div>
  )
}

const AppsCardGrid = lazy(() => import('./AppsCardGrid'))

function CardsFallback() {
  const { t } = useTranslation('appRows')
  return (
    <ul
      role="status"
      aria-label={t('card.loading')}
      className={CARD_GRID_CLASS}
    >
      <AppCardSkeleton />
      <AppCardSkeleton />
      <AppCardSkeleton />
    </ul>
  )
}

export function AppsListBody(props: {
  apps: AppListEntry[]
  mode: ViewMode
  selected: string[]
  onSelect: (name: string, checked: boolean) => void
}) {
  const { data: sites } = useQuery({
    ...staticSitesQueryOptions(),
    retry: false,
  })
  const staticNames = useMemo(
    () => new Set(sites?.map((s) => s.name) ?? []),
    [sites],
  )
  const { data: certs } = useQuery({
    ...certificatesQueryOptions(),
    retry: false,
  })
  const certsByApp = useMemo(() => worstCertByApp(certs ?? []), [certs])
  if (props.mode === 'table') {
    return (
      <AppsTable
        apps={props.apps}
        staticNames={staticNames}
        certsByApp={certsByApp}
        selected={props.selected}
        onSelect={props.onSelect}
      />
    )
  }
  return (
    <Suspense fallback={<CardsFallback />}>
      <AppsCardGrid
        apps={props.apps}
        staticNames={staticNames}
        selected={props.selected}
        onSelect={props.onSelect}
      />
    </Suspense>
  )
}
