import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useDeleteSubmission,
  useSiteSubmissions,
  type SiteSubmission,
} from '../../queries/pagesSubmissions'

const PREVIEW_CHARS = 80

function summary(s: SiteSubmission): string {
  const text = Object.entries(s.fields)
    .map(([k, v]) => `${k}: ${v}`)
    .join(' | ')
  return text.length > PREVIEW_CHARS
    ? `${text.slice(0, PREVIEW_CHARS)}...`
    : text
}

/** Newest form posts across the site's forms, with delete. */
export default function SiteFormsCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useSiteSubmissions(name)
  const del = useDeleteSubmission(name)
  const rows = q.data ?? []

  return (
    <section
      aria-labelledby="pages-forms-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="pages-forms-title" className="text-sm font-semibold">
        {t('forms.title')}
      </h2>
      {q.isPending ? <Skeleton className="h-12 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('forms.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {q.isSuccess && rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <code>/_forms/&lt;name&gt;</code>
        </p>
      ) : null}
      {rows.length > 0 ? (
        <ul className="divide-y divide-border">
          {rows.map((s) => (
            <li
              key={`${s.form}-${s.id}`}
              className="flex items-center gap-3 py-1.5 text-sm"
            >
              <span className="w-24 shrink-0 truncate font-medium">
                {s.form}
              </span>
              <span
                className="min-w-0 flex-1 truncate text-muted-foreground"
                title={summary(s)}
              >
                {summary(s)}
              </span>
              <time
                className="shrink-0 text-xs text-muted-foreground tabular-nums"
                dateTime={s.created_at}
              >
                {new Date(s.created_at).toLocaleString()}
              </time>
              <Button
                size="sm"
                variant="outline"
                disabled={del.isPending}
                aria-label={t('forms.delete', { form: s.form, id: s.id })}
                title={t('forms.deleteTip')}
                onClick={() => del.mutate({ form: s.form, id: s.id })}
              >
                {t('forms.remove')}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}
