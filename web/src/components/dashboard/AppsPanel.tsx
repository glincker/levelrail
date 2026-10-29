import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  LayoutGridGlassIcon,
  ListGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { SkeletonList } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import type { AppListEntry } from '../../types/appDetail'
import { VARIANT_TONE, sortApps } from '../../lib/dashboardApps'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { appListQueryOptions } from '../../queries/apps'
import { AppLogo } from '../apps/AppMetaChips'
import { NewAppMenu } from '../NewAppMenu'
import { FLOATING_CARD } from './cardStyle'
import { clearScope, useScopeName } from './useScopeName'
import { Button } from '@/components/ui/button'

const SHOWN = 12
const VIEW_KEY = 'dashboard.apps.view'
type View = 'grid' | 'list'

function readView(): View {
  try {
    return window.localStorage.getItem(VIEW_KEY) === 'list' ? 'list' : 'grid'
  } catch {
    return 'grid'
  }
}

function imageTag(image: string): string {
  const tag = image.split('@')[0]?.split(':')[1]
  return tag ?? 'latest'
}

function AppTile({ app, list }: { app: AppListEntry; list: boolean }) {
  const tone = VARIANT_TONE[app.status.variant]
  const domain = app.domains?.[0]
  return (
    <li
      className={cn(
        'group/app relative flex items-center gap-3 rounded-lg border border-border bg-card p-3 transition-colors hover:bg-muted/50',
        list && 'rounded-none border-x-0 border-t-0 last:border-b-0',
      )}
    >
      <AppLogo image={app.image} />
      <span className="flex min-w-0 flex-1 flex-col">
        <Link
          to="/apps/$name"
          params={{ name: app.name }}
          className="truncate text-sm font-medium text-foreground after:absolute after:inset-0"
        >
          {app.name}
        </Link>
        <span className="truncate font-mono text-xs text-muted-foreground">
          {domain ?? imageTag(app.image)}
        </span>
      </span>
      <span
        className={cn(
          'inline-flex shrink-0 items-center gap-1.5 text-xs font-medium',
          TONE[tone].text,
        )}
      >
        <span
          aria-hidden="true"
          className={cn('size-2 rounded-full', TONE[tone].solid)}
        />
        {app.status.label}
      </span>
    </li>
  )
}

export function AppsPanel() {
  const { t } = useTranslation('dashboard')
  useEnvironmentScope()
  const scopeName = useScopeName()
  const { data: apps = [], isPending } = useQuery(appListQueryOptions())
  const [query, setQuery] = useState('')
  const [view, setView] = useState<View>(readView)
  const rows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const all = sortApps(apps)
    return q ? all.filter((a) => a.name.toLowerCase().includes(q)) : all
  }, [apps, query])
  const pick = (next: View) => {
    setView(next)
    try {
      window.localStorage.setItem(VIEW_KEY, next)
    } catch {
      // Storage can be blocked; the choice then lasts for this visit only.
    }
  }
  const list = view === 'list'
  return (
    <section
      aria-label={t('apps.title')}
      className={cn('space-y-3 p-4 sm:p-5', FLOATING_CARD)}
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('apps.title')}
        </h2>
        <div className="flex items-center gap-2">
          <span className="relative">
            <SearchGlassIcon
              aria-hidden="true"
              className="pointer-events-none absolute top-1/2 left-2 size-4 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
              }}
              placeholder={t('apps.search')}
              aria-label={t('apps.search')}
              className="h-8 w-44 pl-7"
            />
          </span>
          {(['grid', 'list'] as const).map((v) => (
            <button
              key={v}
              type="button"
              aria-pressed={view === v}
              aria-label={t(`apps.view.${v}`)}
              onClick={() => {
                pick(v)
              }}
              className={cn(
                'rounded-md p-1.5 text-muted-foreground transition-colors hover:text-foreground',
                view === v && 'bg-muted text-foreground',
              )}
            >
              {v === 'grid' ? (
                <LayoutGridGlassIcon className="size-4.5" />
              ) : (
                <ListGlassIcon className="size-4.5" />
              )}
            </button>
          ))}
        </div>
      </div>
      {isPending ? (
        <SkeletonList rows={3} />
      ) : apps.length === 0 && scopeName !== '' ? (
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 rounded-lg bg-muted/40 p-4">
          <div className="min-w-0 space-y-1">
            <p className="text-sm font-medium text-foreground">
              {t('apps.emptyScopedTitle', { scope: scopeName })}
            </p>
            <p className="text-sm text-muted-foreground">
              {t('apps.emptyScopedBody')}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <NewAppMenu />
            <Button size="sm" variant="ghost" onClick={clearScope}>
              {t('apps.emptyScopedClear')}
            </Button>
          </div>
        </div>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('apps.none')}</p>
      ) : (
        <ul
          className={cn(
            list
              ? 'divide-y divide-border'
              : 'grid gap-2 @2xl:grid-cols-2 @5xl:grid-cols-3',
          )}
        >
          {rows.slice(0, SHOWN).map((app) => (
            <AppTile key={app.name} app={app} list={list} />
          ))}
        </ul>
      )}
      {rows.length > SHOWN ? (
        <Link
          to="/apps"
          className="block text-xs text-muted-foreground hover:text-foreground"
        >
          {t('apps.viewAll', { count: rows.length })}
        </Link>
      ) : null}
    </section>
  )
}
