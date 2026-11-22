import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { HelpLink } from '@/components/HelpLink'
import { Input } from '@/components/ui/input'
import { TERM_HELP_PATHS, type TermKey } from '@/components/ui/term'
import { filterGlossary, type GlossaryEntry } from '@/lib/glossary'

// Every Term tooltip links here via #term-<key>, so ids must stay stable.
export function GlossaryList() {
  const { t } = useTranslation('help')
  const { t: tg } = useTranslation('glossary')
  const [query, setQuery] = useState('')

  const entries = useMemo<GlossaryEntry[]>(
    () =>
      (Object.keys(TERM_HELP_PATHS) as TermKey[]).map((key) => ({
        key,
        label: tg(`terms.${key}.label`),
        body: tg(`terms.${key}.body`),
      })),
    [tg],
  )
  const shown = useMemo(() => filterGlossary(entries, query), [entries, query])

  return (
    <section
      id="glossary"
      aria-labelledby="glossary-title"
      className="space-y-3"
    >
      <div>
        <h3 id="glossary-title" className="text-base font-semibold">
          {t('glossary.title')}
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('glossary.description')}
        </p>
      </div>
      <Input
        type="search"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value)
        }}
        placeholder={t('glossary.search')}
        aria-label={t('glossary.searchLabel')}
        className="max-w-sm"
      />
      {shown.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('glossary.empty')}</p>
      ) : (
        <dl className="divide-y divide-border rounded-lg border border-border">
          {shown.map((e) => (
            <div
              key={e.key}
              id={`term-${e.key}`}
              className="scroll-mt-20 p-4 target:bg-muted/50"
            >
              <dt className="text-sm font-semibold text-foreground">
                {e.label}
              </dt>
              <dd className="mt-1 text-sm text-muted-foreground">
                {e.body}
                <HelpLink
                  path={TERM_HELP_PATHS[e.key as TermKey]}
                  label={tg('learnMore')}
                  variant="inline"
                  className="ml-2"
                />
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  )
}
