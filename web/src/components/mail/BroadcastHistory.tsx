import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import {
  useCancelMailBroadcast,
  useMailBroadcast,
  type MailBroadcast,
  type MailBroadcastStatus,
} from '../../queries/mail'

const STATUS_VARIANT: Record<
  MailBroadcastStatus,
  'info' | 'success' | 'muted' | 'destructive' | 'warning'
> = {
  scheduled: 'warning',
  sending: 'info',
  completed: 'success',
  cancelled: 'muted',
  failed: 'destructive',
}

const RECIPIENT_STATUSES = [
  'pending',
  'sent',
  'failed',
  'suppressed',
  'cancelled',
] as const

function DetailsDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useTranslation('mail')
  const { data } = useMailBroadcast(id)
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {data?.broadcast.subject ?? t('messages.history.details')}
          </DialogTitle>
          <DialogDescription>
            {t('messages.history.recipients')}
          </DialogDescription>
        </DialogHeader>
        <ul className="max-h-80 divide-y divide-border overflow-y-auto rounded-lg border border-border">
          {(data?.recipients ?? []).map((r) => (
            <li
              key={r.email}
              className="flex items-center justify-between gap-2 px-3 py-2 text-sm"
            >
              <span className="min-w-0 truncate">
                {r.email}
                {r.error ? (
                  <span className="block text-xs text-destructive">
                    {r.error}
                  </span>
                ) : null}
              </span>
              <Badge
                variant={
                  r.status === 'failed'
                    ? 'destructive'
                    : r.status === 'sent'
                      ? 'success'
                      : 'muted'
                }
              >
                {RECIPIENT_STATUSES.includes(
                  r.status as (typeof RECIPIENT_STATUSES)[number],
                )
                  ? t(
                      `messages.history.recipientStatus.${r.status as (typeof RECIPIENT_STATUSES)[number]}`,
                    )
                  : r.status}
              </Badge>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  )
}

/** BroadcastHistory lists sent and scheduled messages with live progress, details and cancel. */
export function BroadcastHistory({
  broadcasts,
}: {
  broadcasts: MailBroadcast[]
}) {
  const { t } = useTranslation('mail')
  const cancel = useCancelMailBroadcast()
  const [details, setDetails] = useState<string | null>(null)
  const [stopping, setStopping] = useState<MailBroadcast | null>(null)

  return (
    <section aria-label={t('messages.history.title')} className="space-y-3">
      <h2 className="text-sm font-medium">{t('messages.history.title')}</h2>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {broadcasts.map((b) => {
          const done = b.sent + b.failed + b.suppressed
          const active = b.status === 'sending' || b.status === 'scheduled'
          return (
            <li key={b.id} className="space-y-2 px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="flex min-w-0 flex-wrap items-center gap-2 text-sm font-medium">
                  <span className="truncate">{b.subject}</span>
                  <Badge variant={STATUS_VARIANT[b.status]}>
                    {t(`messages.status.${b.status}`)}
                  </Badge>
                </p>
                <div className="flex gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setDetails(b.id)}
                  >
                    {t('messages.history.details')}
                  </Button>
                  {active ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      writeAction
                      onClick={() => setStopping(b)}
                    >
                      {t('messages.history.cancel')}
                    </Button>
                  ) : null}
                </div>
              </div>
              <Progress
                value={b.total === 0 ? 0 : Math.round((done / b.total) * 100)}
                aria-label={t('messages.history.columns.progress')}
              />
              <p className="text-xs text-muted-foreground">
                {t('messages.history.progress', {
                  sent: b.sent,
                  failed: b.failed,
                  suppressed: b.suppressed,
                  total: b.total,
                })}
                {' - '}
                {new Date(
                  b.status === 'scheduled' && b.scheduled_at
                    ? b.scheduled_at
                    : b.created_at,
                ).toLocaleString()}
              </p>
            </li>
          )
        })}
      </ul>
      {details ? (
        <DetailsDialog id={details} onClose={() => setDetails(null)} />
      ) : null}
      <ConfirmDialog
        open={stopping !== null}
        onOpenChange={(o) => {
          if (!o) setStopping(null)
        }}
        title={t('messages.history.cancelTitle')}
        description={t('messages.history.cancelDescription')}
        tone="destructive"
        confirmLabel={t('messages.history.cancel')}
        pending={cancel.isPending}
        onConfirm={() => {
          if (!stopping) return
          cancel.mutate(stopping.id, {
            onSuccess: () => {
              toast.add({
                title: t('messages.history.cancelled'),
                type: 'success',
              })
              setStopping(null)
            },
            onError: (e) => toast.add({ title: e.message, type: 'error' }),
          })
        }}
      />
    </section>
  )
}
