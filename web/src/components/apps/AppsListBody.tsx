import { lazy, Suspense, useMemo, useState } from 'react'
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
import { AppsGroupHeader } from './AppsGroupHeader'
import { groupApps, type GroupBy } from '../../lib/appsGroup'
import { useProjectListOptional } from '../../queries/projects'

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

function useGroups(apps: AppListEntry[], by: GroupBy) {
  const { data: projects } = useProjectListOptional()
  return useMemo(() => {
    if (by === 'none') return null
    const names = new Map((projects ?? []).map((p) => [p.id, p.name]))
    return groupApps(apps, by, names)
  }, [apps, by, projects])
}

export function AppsListBody(props: {
  apps: AppListEntry[]
  mode: ViewMode
  selected: string[]
  onSelect: (name: string, checked: boolean) => void
  groupBy?: GroupBy
  onSelectMany?: (names: string[], checked: boolean) => void
}) {
  const { t } = useTranslation('appRows')
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const groups = useGroups(props.apps, props.groupBy ?? 'none')
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
  const body = (apps: AppListEntry[]) =>
    props.mode === 'table' ? (
      <AppsTable
        apps={apps}
        staticNames={staticNames}
        certsByApp={certsByApp}
        selected={props.selected}
        onSelect={props.onSelect}
      />
    ) : (
      <Suspense fallback={<CardsFallback />}>
        <AppsCardGrid
          apps={apps}
          staticNames={staticNames}
          selected={props.selected}
          onSelect={props.onSelect}
        />
      </Suspense>
    )
  if (!groups || props.groupBy === undefined || props.groupBy === 'none') {
    return body(props.apps)
  }
  const none = t(
    props.groupBy === 'node'
      ? 'group.noneNode'
      : props.groupBy === 'project'
        ? 'group.noneProject'
        : 'group.noneEnvironment',
  )
  return (
    <div className="space-y-6">
      {groups.map((g) => {
        const names = g.apps.map((a) => a.name)
        const isCollapsed = collapsed.has(g.key)
        return (
          <section key={g.key} className="space-y-3">
            <AppsGroupHeader
              label={g.label === '' ? none : g.label}
              count={g.apps.length}
              selectedCount={
                names.filter((n) => props.selected.includes(n)).length
              }
              collapsed={isCollapsed}
              onToggle={() => {
                setCollapsed((prev) => {
                  const next = new Set(prev)
                  if (!next.delete(g.key)) next.add(g.key)
                  return next
                })
              }}
              onSelectGroup={(checked) => {
                props.onSelectMany?.(names, checked)
              }}
            />
            {isCollapsed ? null : body(g.apps)}
          </section>
        )
      })}
    </div>
  )
}
