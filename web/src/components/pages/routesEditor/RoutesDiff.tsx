import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { DraftRedirect } from '../rulesDraft'
import { diffLines, redirectsToText } from './rawText'

/** Saved vs draft rules, one line per rule; unchanged lines are dimmed. */
export function RoutesDiff({
  saved,
  draft,
}: Readonly<{
  saved: readonly DraftRedirect[]
  draft: readonly DraftRedirect[]
}>) {
  const { t } = useTranslation('pagesSites')
  const split = (rows: readonly DraftRedirect[]) =>
    rows.length ? redirectsToText(rows).split('\n') : []
  const ops = diffLines(split(saved), split(draft))
  const changed = ops.some((o) => o.kind !== 'same')
  return (
    <div
      role="group"
      aria-label={t('routes.diff.title')}
      className="rounded-md border border-border bg-muted/30 p-2 font-mono text-xs"
    >
      {changed ? (
        <ul>
          {ops.map((o, i) => (
            <li
              key={`${i}:${o.kind}:${o.line}`}
              className={cn(
                'break-all px-1',
                o.kind === 'add' && 'bg-success/15 text-success',
                o.kind === 'del' && 'bg-destructive/15 text-destructive',
                o.kind === 'same' && 'text-muted-foreground',
              )}
            >
              <span aria-hidden="true">
                {o.kind === 'add' ? '+ ' : o.kind === 'del' ? '- ' : '  '}
              </span>
              <span className="sr-only">{t(`routes.diff.${o.kind}`)}</span>
              {o.line}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-muted-foreground">{t('routes.diff.none')}</p>
      )}
    </div>
  )
}
