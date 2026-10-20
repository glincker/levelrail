import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  accessReviewQueryOptions,
  useCompleteReview,
  useDecideItem,
  type AccessDecision,
  type AccessReviewItem,
} from '../../queries/compliance'
import { formatDate } from '../../lib/format'

function ItemRow({
  item,
  locked,
  onDecide,
}: Readonly<{
  item: AccessReviewItem
  locked: boolean
  onDecide: (decision: AccessDecision) => void
}>) {
  const { t } = useTranslation('compliance')
  return (
    <li className="flex flex-wrap items-center justify-between gap-3 px-3 py-2">
      <div className="min-w-0 space-y-0.5">
        <p className="truncate text-sm font-medium">{item.label}</p>
        <p className="text-xs text-muted-foreground">
          {t(`kind.${item.kind}`)}
          {item.role ? ` · ${item.role}` : ''} · {t('review.lastActive')}:{' '}
          {formatDate(item.last_active_at, t('review.never'))}
        </p>
        {item.apply_error ? (
          <p role="alert" className="text-xs text-destructive">
            {item.apply_error}
          </p>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        {item.applied_at ? (
          <Badge variant="success">{t('review.revoked')}</Badge>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant={item.decision === 'keep' ? 'default' : 'outline'}
          disabled={locked || Boolean(item.applied_at)}
          aria-pressed={item.decision === 'keep'}
          onClick={() => onDecide('keep')}
        >
          {t('review.keep')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant={item.decision === 'revoke' ? 'destructive' : 'outline'}
          disabled={locked || Boolean(item.applied_at)}
          aria-pressed={item.decision === 'revoke'}
          onClick={() => onDecide('revoke')}
        >
          {t('review.revoke')}
        </Button>
      </div>
    </li>
  )
}

export function AccessReviewDetailView({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('compliance')
  const { data } = useQuery(accessReviewQueryOptions(id))
  const decide = useDecideItem(id)
  const complete = useCompleteReview(id)
  const [confirm, setConfirm] = useState(false)
  if (!data) return null
  const locked = data.status === 'completed'
  const pending = data.total - data.decided
  const runComplete = () =>
    complete.mutate(undefined, {
      onSuccess: (res) => {
        setConfirm(false)
        if (res.failures.length > 0) {
          toast.add({
            title: t('review.partial', { count: res.failures.length }),
            type: 'error',
          })
          return
        }
        toast.add({
          title: t('review.completed', { count: res.revoked }),
          type: 'success',
        })
      },
      onError: (err) => toast.add({ title: err.message, type: 'error' }),
    })
  return (
    <div className="space-y-3">
      <ul className="divide-y divide-border rounded-lg border border-border">
        {data.items.map((item) => (
          <ItemRow
            key={item.id}
            item={item}
            locked={locked}
            onDecide={(decision) =>
              decide.mutate(
                { item: item.id, decision },
                {
                  onError: (err) =>
                    toast.add({ title: err.message, type: 'error' }),
                },
              )
            }
          />
        ))}
      </ul>
      {locked ? null : (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">
            {t('review.progress', {
              decided: data.decided,
              total: data.total,
              revoke: data.to_revoke,
            })}
          </p>
          <Button
            type="button"
            disabled={pending > 0}
            onClick={() => setConfirm(true)}
          >
            {t('review.complete')}
          </Button>
        </div>
      )}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('review.confirmTitle')}
        description={t('review.confirmDescription', { count: data.to_revoke })}
        confirmLabel={t('review.complete')}
        pendingLabel={t('review.completing')}
        tone={data.to_revoke > 0 ? 'destructive' : 'default'}
        pending={complete.isPending}
        onConfirm={runComplete}
      />
    </div>
  )
}
