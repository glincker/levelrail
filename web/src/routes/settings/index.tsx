import { useMemo, useState } from 'react'
import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { filterSettingsNav, useSettingsNav } from '../../hooks/useSettingsNav'
import { PageHeader } from '../../components/shell/PageHeader'
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

function SettingsHubPage() {
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')
  const visible = useMemo(
    () => filterSettingsNav(sections, query),
    [sections, query],
  )

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('nav.title')}
        description={t('nav.description')}
        actions={
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
              className="rounded-full border border-border px-3 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
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
              <Link key={item.to} to={item.to} className="block">
                <Card className="h-full rounded-[var(--glinui-radius-lg)] transition-colors hover:ring-foreground/20">
                  <CardHeader>
                    <CardTitle className="flex items-center gap-2">
                      <item.icon className="size-4" />
                      {item.title}
                    </CardTitle>
                    <CardDescription>{item.description}</CardDescription>
                  </CardHeader>
                </Card>
              </Link>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
