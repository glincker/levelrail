import { useMemo, useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  TriangleAlertGlassIcon,
  SearchGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'
import {
  filterSettingsNav,
  mergeSettingsHits,
  useSettingsNav,
  type LabelledSettingsItem,
} from '../../hooks/useSettingsNav'
import { useSearch } from '../../queries/search'
import { Badge } from '@/components/ui/badge'
import { BETA_FEATURES_PATH, lockedSearch } from '../../lib/experimental'
import { PageBanner } from '../../components/PageBanner'
import { EmptyState } from '../../components/ui/empty-state'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Skeleton } from '../../components/ui/skeleton'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
} from '../../components/ui/card'

export const Route = createFileRoute('/settings/')({
  component: SettingsHubPage,
})

function HubCardLink({ item }: { item: LabelledSettingsItem }) {
  const { t } = useTranslation()
  const card = (
    <Card className="h-full rounded-[var(--glinui-radius-lg)] p-0 transition-colors hover:bg-muted/40 hover:ring-foreground/20">
      <CardHeader className="gap-1 p-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium">
          <item.icon className="size-5" />
          {item.title}
          {item.locked ? (
            <Badge variant="muted">{t('shell.locked.badge')}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription className="line-clamp-2 text-[13px]">
          {item.description}
        </CardDescription>
      </CardHeader>
    </Card>
  )
  if (item.locked && item.feature) {
    return (
      <Link
        to={BETA_FEATURES_PATH}
        search={lockedSearch(item.feature)}
        className="block"
        data-locked="true"
      >
        {card}
      </Link>
    )
  }
  return (
    <Link to={item.to} className="block">
      {card}
    </Link>
  )
}

function SettingsHubPage() {
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')
  const q = query.trim()
  const settingHits = useSearch(
    { q: `kind:setting ${q}`, limit: 30 },
    q.length >= 2,
  )
  const visible = useMemo(() => {
    const local = filterSettingsNav(sections, query)
    if (q.length < 2) return local
    const hrefs = new Set<string>()
    for (const hit of settingHits.data?.hits ?? []) {
      if (hit.href) hrefs.add(hit.href)
    }
    return mergeSettingsHits(sections, local, hrefs)
  }, [sections, query, q, settingHits.data])

  const searching = q.length >= 2 && settingHits.isLoading
  const searchFailed = q.length >= 2 && settingHits.isError

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<SettingsGlassIcon size="1.4em" />}
        titleKey="settings:nav.title"
        descriptionKey="settings:nav.description"
        size="compact"
        action={
          <div className="relative w-full sm:w-64">
            <SearchGlassIcon
              className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              value={query}
              onChange={(event) => {
                setQuery(event.target.value)
              }}
              placeholder={t('nav.search')}
              aria-label={t('nav.search')}
              className="h-8 pl-9"
            />
          </div>
        }
      />

      {searching ? (
        <div
          role="status"
          aria-busy="true"
          aria-label={t('nav.searchLoading')}
          className="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-3"
        >
          {Array.from({ length: 3 }, (_, i) => (
            <Skeleton key={i} className="h-20 w-full" />
          ))}
        </div>
      ) : null}

      {searchFailed ? (
        <EmptyState
          kind="error"
          icon={<TriangleAlertGlassIcon className="size-5" />}
          title={t('nav.searchFailedTitle')}
          description={t('nav.searchFailedDescription')}
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void settingHits.refetch()
              }}
            >
              {t('nav.retry')}
            </Button>
          }
        />
      ) : null}

      {visible.length === 0 ? (
        <EmptyState
          icon={<SearchGlassIcon className="size-6" />}
          title={t('nav.noMatchTitle', { query: query.trim() })}
          description={t('nav.noMatchDescription')}
        />
      ) : null}

      {visible.map((section) => (
        <section
          key={section.id}
          id={`settings-${section.id}`}
          className="scroll-mt-20 space-y-2 pt-3"
        >
          <div className="flex items-baseline justify-between gap-3">
            <div>
              <h2 className="text-sm font-medium text-foreground">
                {section.title}
              </h2>
              <p className="text-xs text-muted-foreground">
                {section.description}
              </p>
            </div>
            <span className="text-xs text-muted-foreground tabular-nums">
              {section.items.length}
            </span>
          </div>
          <div className="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-3">
            {section.items.map((item) => (
              <HubCardLink key={item.to} item={item} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
