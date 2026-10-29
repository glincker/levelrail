import { useTranslation } from 'react-i18next'
import { BellOffGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import type { AttentionItem } from '../../lib/attention'
import {
  isSilenceActive,
  useDeleteSilence,
  type IssueSilence,
} from '../../queries/silences'
import { unsnoozeNow } from '../shell/snoozeStore'

// Muted issues stay listed here, collapsed, with who silenced them and until
// when. A silenced critical issue keeps its destructive tone, only softer.
export function SilencedSection({
  items,
  silences,
  now,
  isSnoozed,
}: {
  items: AttentionItem[]
  silences: IssueSilence[]
  now: number
  isSnoozed: (id: string) => boolean
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const remove = useDeleteSilence()
  const active = silences.filter((s) => isSilenceActive(s, now))
  const itemById = new Map(items.map((i) => [i.id, i]))
  const legacy = items.filter(
    (i) => isSnoozed(i.id) && !active.some((s) => s.issue_key === i.id),
  )
  const count = active.length + legacy.length
  if (count === 0) return null

  return (
    <details className="rounded-lg border border-border bg-card/50 p-3">
      <summary className="flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground">
        <BellOffGlassIcon className="size-4.5" aria-hidden="true" />
        {t('silence.sectionTitle', { count })}
      </summary>
      <ul className="mt-2 space-y-2">
        {active.map((s) => {
          const item = itemById.get(s.issue_key)
          const critical = (item?.severity ?? s.severity) === 'critical'
          return (
            <li
              key={s.id}
              className="flex flex-wrap items-center justify-between gap-2 text-xs"
            >
              <div className="min-w-0">
                <p
                  className={`font-medium break-words ${critical ? 'text-destructive/70' : 'text-muted-foreground'}`}
                >
                  {item?.title ?? s.issue_key}
                </p>
                <p className="break-words text-muted-foreground">
                  {t('silence.by', { who: s.created_by })},{' '}
                  {s.until
                    ? t('silence.until', {
                        when: new Date(s.until).toLocaleString(),
                      })
                    : t('silence.forever')}
                  {s.reason ? `: ${s.reason}` : ''}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={remove.isPending}
                onClick={() => {
                  remove.mutate(s.id, {
                    onSuccess: () => {
                      toast.add({
                        title: t('silence.unsilenced', {
                          title: item?.title ?? s.issue_key,
                        }),
                        type: 'info',
                      })
                    },
                    onError: (err) => {
                      toast.add({ title: err.message, type: 'error' })
                    },
                  })
                }}
              >
                {t('silence.unsilence')}
              </Button>
            </li>
          )
        })}
        {legacy.map((i) => (
          <li
            key={i.id}
            className="flex flex-wrap items-center justify-between gap-2 text-xs"
          >
            <p className="font-medium text-muted-foreground">{i.title}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                unsnoozeNow(i.id)
              }}
            >
              {t('silence.unsilence')}
            </Button>
          </li>
        ))}
      </ul>
    </details>
  )
}
