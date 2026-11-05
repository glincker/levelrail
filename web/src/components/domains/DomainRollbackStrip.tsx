import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  domainChangesQueryOptions,
  useRollbackDomainChange,
  type DomainChange,
  type RollbackResult,
} from '../../queries/domainChanges'

const MINUTE_MS = 60_000
const MINUTES_PER_HOUR = 60

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, MINUTE_MS)
    return () => {
      window.clearInterval(id)
    }
  }, [])
  return now
}

function RollbackOutcome({ result }: { result: RollbackResult }) {
  const { t } = useTranslation('domains')
  return (
    <ul className="space-y-0.5 text-xs" aria-label={t('rollback.outcomes')}>
      {result.results.map((r) => (
        <li key={`${r.type}-${r.name}`} className="flex flex-wrap gap-x-2">
          <span className="font-mono">
            {r.type} {r.name}
          </span>
          <span className="text-muted-foreground">
            {r.outcome === 'skipped'
              ? t('rollback.skipped')
              : r.message || r.outcome}
          </span>
        </li>
      ))}
    </ul>
  )
}

// Shows the newest change that can still be undone, with how long is left.
export function DomainRollbackStrip({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const now = useNow()
  const { data } = useQuery(domainChangesQueryOptions(domain))
  const rollback = useRollbackDomainChange(domain)
  const [open, setOpen] = useState(false)
  const [done, setDone] = useState<RollbackResult | null>(null)

  const changes = [...(data ?? [])].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  )
  const live: DomainChange | undefined = changes.find(
    (c) => c.can_rollback && Date.parse(c.rollback_until) > now,
  )
  const latest = live ?? changes[0]
  if (!latest && !done) return null

  const minutes = live
    ? Math.max(
        1,
        Math.ceil((Date.parse(live.rollback_until) - now) / MINUTE_MS),
      )
    : 0
  const left =
    minutes >= 2 * MINUTES_PER_HOUR
      ? t('rollback.hoursLeft', {
          count: Math.floor(minutes / MINUTES_PER_HOUR),
        })
      : t('rollback.minutesLeft', { count: minutes })

  return (
    <div className="space-y-2 rounded-md border border-border bg-muted/30 p-3 text-sm">
      {latest ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0 space-y-0.5">
            <p className="font-medium break-words">{latest.summary}</p>
            <p className="text-xs text-muted-foreground">
              {live
                ? left
                : latest.already_undone
                  ? t('rollback.undone')
                  : t('rollback.closed')}
            </p>
          </div>
          {live ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                rollback.reset()
                setOpen(true)
              }}
            >
              {t('rollback.button')}
            </Button>
          ) : null}
        </div>
      ) : null}
      {done ? <RollbackOutcome result={done} /> : null}
      {live ? (
        <ConfirmDialog
          open={open}
          onOpenChange={setOpen}
          title={t('rollback.confirmTitle')}
          description={live.summary}
          consequences={[
            t('rollback.consequenceRemove'),
            t('rollback.consequenceRestore'),
            t('rollback.consequenceSkip'),
          ]}
          confirmLabel={t('rollback.confirm')}
          pendingLabel={t('rollback.pending')}
          tone="destructive"
          pending={rollback.isPending}
          error={rollback.error?.message ?? null}
          onConfirm={() => {
            rollback.mutate(live.run_id, {
              onSuccess: (res) => {
                setDone(res)
                setOpen(false)
              },
            })
          }}
        />
      ) : null}
    </div>
  )
}
