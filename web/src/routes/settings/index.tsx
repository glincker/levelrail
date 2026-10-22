import { useMemo, useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GearIcon, MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Input } from '../../components/ui/input'
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
    <Card className="h-full rounded-[var(--glinui-radius-lg)] transition-colors hover:ring-foreground/20">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <item.icon className="size-4" />
          {item.title}
          {item.locked ? (
            <Badge variant="muted">{t('shell.locked.badge')}</Badge>
          ) : null}
        </CardTitle>
        <CardDescription>{item.description}</CardDescription>
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

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<GearIcon />}
        titleKey="settings:nav.title"
        descriptionKey="settings:nav.description"
        size="compact"
        action={
          <div className="relative w-full sm:w-64">
            <MagnifyingGlassIcon
              className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              value={query}
              onChange={(event) => {
                setQuery(event.target.value)
              }}
              placeholder={t('nav.search')}
              aria-label={t('nav.search')}
              className="h-8 pl-7"
            />
          </div>
        }
      />

      {query.trim() === '' ? (
        <nav aria-label={t('nav.jumpTo')} className="flex flex-wrap gap-2">
          {sections.map((section) => (
            <a
              key={section.id}
              href={`#settings-${section.id}`}
              className="rounded-full border border-border px-3 py-1 text-xs pointer-coarse:py-2.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {section.title}
            </a>
          ))}
        </nav>
      ) : null}

      {visible.length === 0 ? (
        <EmptyState
          icon={<MagnifyingGlassIcon className="size-5" />}
          title={t('nav.noMatchTitle', { query: query.trim() })}
          description={t('nav.noMatchDescription')}
        />
      ) : null}

      {visible.map((section) => (
        <section
          key={section.id}
          id={`settings-${section.id}`}
          className="scroll-mt-20 space-y-3"
        >
          <div>
            <h2 className="text-sm font-medium text-foreground">
              {section.title}
            </h2>
            <p className="text-xs text-muted-foreground">
              {section.description}
            </p>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {section.items.map((item) => (
              <HubCardLink key={item.to} item={item} />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
