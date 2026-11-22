import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { RelativeTime } from '@/components/kit'
import { usePreviewEnvironments } from '../../queries/previewEnvironments'

/** Newest previews shown on the overview. */
const SHOWN = 4

const DOT: Record<string, string> = {
  active: 'bg-success',
  deploying: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

/** Latest preview environments with a calm empty line so the column never collapses. */
export function AppRecentPreviews({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('appOverview')
  const rows = [...(usePreviewEnvironments(name).data ?? [])]
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    .slice(0, SHOWN)
  return (
    <section
      aria-labelledby="app-recent-previews-title"
      className="space-y-1.5 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="app-recent-previews-title" className="text-sm font-semibold">
          {t('previews.title')}
        </h2>
        <Link
          to="/apps/$name/previews"
          params={{ name }}
          className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
        >
          {t('previews.all')}
        </Link>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('previews.none')}</p>
      ) : (
        <ul className="divide-y divide-border">
          {rows.map((p) => (
            <li
              key={p.pr_number}
              className="flex min-w-0 items-center gap-2 py-1 text-xs"
            >
              <span
                aria-hidden="true"
                className={`size-2 shrink-0 rounded-full ${DOT[p.status] ?? 'bg-muted-foreground/50'}`}
              />
              {p.domain ? (
                <a
                  href={`https://${p.domain}`}
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
                <RelativeTime at={p.updated_at} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
