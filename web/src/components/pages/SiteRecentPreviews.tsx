import { useTranslation } from 'react-i18next'
import { formatAge } from '../../lib/format'
import { usePagePreviews } from '../../queries/pagePreviews'

/** Newest previews shown on the overview. */
const SHOWN = 4

const DOT: Record<string, string> = {
  active: 'bg-success',
  deploying: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

/** Latest preview of each branch, with a calm empty line so the column never collapses. */
export function SiteRecentPreviews({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const rows = (usePagePreviews(name).data ?? []).slice(0, SHOWN)
  return (
    <section
      aria-labelledby="pages-recent-previews-title"
      className="space-y-1.5 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="pages-recent-previews-title" className="text-sm font-semibold">
        {t('overview.previews.title')}
      </h2>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('overview.previews.none')}
        </p>
      ) : (
        <ul className="divide-y divide-border">
          {rows.map((p) => (
            <li
              key={p.site}
              className="flex min-w-0 items-center gap-2 py-1 text-xs"
            >
              <span
                aria-hidden="true"
                className={`size-2 shrink-0 rounded-full ${DOT[p.status] ?? 'bg-muted-foreground/50'}`}
              />
              {p.url ? (
                <a
                  href={p.url}
                  target="_blank"
                  rel="noreferrer"
                  className="min-w-0 flex-1 truncate font-medium underline-offset-2 hover:underline focus-visible:underline"
                >
                  {p.branch}
                </a>
              ) : (
                <span className="min-w-0 flex-1 truncate font-medium">
                  {p.branch}
                </span>
              )}
              <span className="shrink-0 text-muted-foreground">
                {formatAge(p.updated_at)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
