import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  BanGlassIcon,
  DownloadGlassIcon,
  InboxGlassIcon,
  SearchGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from '../kit/RelativeTime'
import {
  exportUrl,
  useFormActions,
  useFormInbox,
  type FormSubmission,
  type SpamFilter,
} from '../../queries/forms'

const FILTERS: SpamFilter[] = ['inbox', 'spam', 'all']
const ROW_ESTIMATE = 96
const SHOWN_FIELDS = 5

function SubmissionRow({
  s,
  onSpam,
  onRemove,
}: {
  s: FormSubmission
  onSpam: (s: FormSubmission) => void
  onRemove: (s: FormSubmission) => void
}) {
  const { t } = useTranslation('forms')
  const entries = Object.entries(s.fields)
  return (
    <div className="flex items-start gap-3 border-b border-border px-4 py-3">
      <dl className="min-w-0 flex-1 space-y-0.5 text-sm">
        {entries.slice(0, SHOWN_FIELDS).map(([k, v]) => (
          <div key={k} className="flex gap-2">
            <dt className="shrink-0 text-muted-foreground">{k}</dt>
            <dd className="min-w-0 break-words">{v}</dd>
          </div>
        ))}
        {entries.length > SHOWN_FIELDS && (
          <p className="text-xs text-muted-foreground">
            +{entries.length - SHOWN_FIELDS}
          </p>
        )}
      </dl>
      <div className="flex shrink-0 flex-col items-end gap-1.5">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {s.test && <Badge variant="outline">{t('inbox.testBadge')}</Badge>}
          {s.spam && (
            <Badge variant="warning">
              {s.spam_reason
                ? t(`inbox.reason.${s.spam_reason}`, {
                    defaultValue: t('inbox.spamBadge'),
                  })
                : t('inbox.spamBadge')}
            </Badge>
          )}
          <RelativeTime at={s.created_at} />
        </div>
        <div className="flex gap-1">
          <Button
            writeAction
            variant="ghost"
            size="xs"
            onClick={() => onSpam(s)}
          >
            <BanGlassIcon size="1.2em" />
            {s.spam ? t('inbox.markNotSpam') : t('inbox.markSpam')}
          </Button>
          <Button
            writeAction
            variant="ghost"
            size="xs"
            aria-label={t('inbox.remove')}
            onClick={() => onRemove(s)}
          >
            <TrashGlassIcon size="1.2em" />
          </Button>
        </div>
      </div>
    </div>
  )
}

export function FormInbox({ app, form }: { app: string; form: string }) {
  const { t } = useTranslation('forms')
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<SpamFilter>('inbox')
  const inbox = useFormInbox(app, form, q, filter)
  const { setSpam, removeSubmission } = useFormActions(app)
  const parentRef = useRef<HTMLDivElement>(null)

  const rows = useMemo(
    () => inbox.data?.pages.flatMap((p) => p.submissions) ?? [],
    [inbox.data],
  )
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_ESTIMATE,
    overscan: 6,
  })

  const fail = () =>
    toast.add({ title: t('inbox.actionFailed'), type: 'error' })
  const toggleSpam = (s: FormSubmission) =>
    setSpam.mutate({ form, id: s.id, spam: !s.spam }, { onError: fail })
  const remove = (s: FormSubmission) => {
    if (!window.confirm(t('inbox.removeConfirm'))) return
    removeSubmission.mutate({ form, id: s.id }, { onError: fail })
  }

  return (
    <section aria-label={t('inbox.title')} className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-48 flex-1">
          <SearchGlassIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('inbox.search')}
            aria-label={t('inbox.search')}
            className="pl-8"
          />
        </div>
        <div role="group" aria-label={t('inbox.title')} className="flex gap-1">
          {FILTERS.map((f) => (
            <Button
              key={f}
              size="sm"
              variant={filter === f ? 'secondary' : 'ghost'}
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
            >
              {t(
                f === 'all'
                  ? 'inbox.filterAll'
                  : f === 'inbox'
                    ? 'inbox.filterInbox'
                    : 'inbox.filterSpam',
              )}
            </Button>
          ))}
        </div>
        <a
          href={exportUrl(app, form, q, filter)}
          download
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          <DownloadGlassIcon size="1.2em" />
          {t('inbox.export')}
        </a>
      </div>

      {inbox.isPending && (
        <p role="status" className="text-sm text-muted-foreground">
          {t('inbox.loading')}
        </p>
      )}
      {inbox.isError && (
        <p role="alert" className="text-sm text-destructive">
          {t('inbox.loadError')}
        </p>
      )}
      {inbox.isSuccess && rows.length === 0 && (
        <EmptyState
          compact
          icon={<InboxGlassIcon className="size-5.5" />}
          title={t(
            q || filter !== 'inbox' ? 'inbox.noMatchTitle' : 'inbox.emptyTitle',
          )}
          description={t(
            q || filter !== 'inbox'
              ? 'inbox.noMatchDescription'
              : 'inbox.emptyDescription',
          )}
        />
      )}
      {rows.length > 0 && (
        <div
          ref={parentRef}
          className="max-h-[32rem] overflow-auto rounded-lg border border-border bg-card"
        >
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((v) => {
              const s = rows[v.index]
              if (!s) return null
              return (
                <div
                  key={s.id}
                  data-index={v.index}
                  ref={virtualizer.measureElement}
                  className="absolute top-0 left-0 w-full"
                  style={{ transform: `translateY(${v.start}px)` }}
                >
                  <SubmissionRow s={s} onSpam={toggleSpam} onRemove={remove} />
                </div>
              )
            })}
          </div>
        </div>
      )}
      {inbox.hasNextPage && (
        <Button
          variant="outline"
          size="sm"
          disabled={inbox.isFetchingNextPage}
          onClick={() => void inbox.fetchNextPage()}
        >
          {t('inbox.loadMore')}
        </Button>
      )}
    </section>
  )
}
