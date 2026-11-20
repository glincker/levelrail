import { useTranslation } from 'react-i18next'
import { formatAge } from '../../lib/format'
import { usePagePreviews } from '../../queries/pagePreviews'

const SHORT_SHA = 7

const DOT: Record<string, string> = {
  active: 'bg-success',
  deploying: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

/** Latest preview deploy of each branch; hidden until the site has one. */
export default function SitePreviewsCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const { data } = usePagePreviews(name)
  if (!data || data.length === 0) return null
  return (
    <section
      aria-labelledby="pages-previews-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="pages-previews-title" className="text-sm font-semibold">
        {t('previews.title')}
      </h2>
      <ul className="divide-y divide-border">
        {data.map((p) => (
          <li
            key={p.site}
            className="flex min-w-0 items-center gap-3 py-1.5 text-sm"
          >
            <span
              aria-hidden="true"
              className={`size-2 shrink-0 rounded-full ${DOT[p.status] ?? 'bg-muted-foreground'}`}
            />
            <span className="min-w-0 flex-1 truncate font-medium">
              {p.branch}
            </span>
            <span className="hidden font-mono text-xs text-muted-foreground sm:inline">
              {p.commit_sha?.slice(0, SHORT_SHA)}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {formatAge(p.updated_at)}
            </span>
            {p.url ? (
              <a
                href={p.url}
                target="_blank"
                rel="noreferrer"
                className="shrink-0 text-xs underline underline-offset-2"
              >
                {t('previews.open')}
              </a>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}
