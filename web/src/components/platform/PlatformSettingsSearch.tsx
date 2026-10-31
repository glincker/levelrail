import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  filterPlatformSettings,
  PLATFORM_PAGE_PATH,
  type PlatformSettingId,
} from '../../lib/platformSettingsIndex'

function isTypingTarget(el: EventTarget | null): boolean {
  if (!(el instanceof HTMLElement)) return false
  return (
    el.isContentEditable ||
    el.tagName === 'INPUT' ||
    el.tagName === 'TEXTAREA' ||
    el.tagName === 'SELECT'
  )
}

// Flat search across every platform setting; "/" focuses it, Enter opens
// the first hit at its card.
export function PlatformSettingsSearch() {
  const { t } = useTranslation('platformSettings')
  const navigate = useNavigate()
  const inputRef = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return
      if (isTypingTarget(e.target)) return
      e.preventDefault()
      inputRef.current?.focus()
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
    }
  }, [])

  const text = (id: PlatformSettingId) => ({
    title: t(`index.${id}.title`),
    description: t(`index.${id}.description`),
  })
  const results = useMemo(
    () => filterPlatformSettings(query, text),
    // text closes over t, which changes with the language
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [query, t],
  )

  return (
    <section aria-labelledby="platform-search-label" className="space-y-2">
      <Label id="platform-search-label" htmlFor="platform-search">
        {t('overview.search.label')}
      </Label>
      <Input
        id="platform-search"
        ref={inputRef}
        type="search"
        value={query}
        placeholder={t('overview.search.placeholder')}
        aria-describedby="platform-search-hint"
        onChange={(e) => {
          setQuery(e.target.value)
        }}
        onKeyDown={(e) => {
          const first = results[0]
          if (e.key !== 'Enter' || !first) return
          e.preventDefault()
          void navigate({
            to: PLATFORM_PAGE_PATH[first.page],
            hash: first.anchor,
          })
        }}
      />
      <p id="platform-search-hint" className="text-xs text-muted-foreground">
        {t('overview.search.hint')}
      </p>
      <p role="status" className="sr-only">
        {t('overview.search.count', { count: results.length })}
      </p>
      {results.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('overview.search.empty', { query })}
        </p>
      ) : (
        <ul
          aria-label={t('overview.search.results')}
          className="divide-y divide-border rounded-lg border border-border bg-card"
        >
          {results.map((entry) => {
            const { title, description } = text(entry.id)
            const page = t(`pageNames.${entry.page}`)
            return (
              <li key={entry.id}>
                <Link
                  to={PLATFORM_PAGE_PATH[entry.page]}
                  hash={entry.anchor}
                  aria-label={t('overview.search.go', { title, page })}
                  className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 hover:bg-muted/60 focus-visible:bg-muted/60"
                >
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium text-foreground">
                      {title}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {description}
                    </span>
                  </span>
                  <span className="text-xs text-muted-foreground">{page}</span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
