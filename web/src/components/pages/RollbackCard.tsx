import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { formatAge } from '../../lib/format'
import { useRollbackToDeploy } from '../../queries/deployControl'
import type { DeployAttempt } from '../../types/deployAttempt'

/** The newest successful deploy that is not live, highlighted with a one-click rollback. */
export function RollbackCard({
  name,
  attempts,
}: Readonly<{ name: string; attempts: DeployAttempt[] }>) {
  const { t } = useTranslation('pagesSites')
  const rollback = useRollbackToDeploy(name)
  const [confirming, setConfirming] = useState(false)
  const prev = attempts.find((a) => a.status === 'succeeded' && !a.live)
  if (!prev) return null
  const label = prev.commit_sha?.slice(0, 7) ?? prev.id.slice(0, 8)
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-brand/40 bg-muted/40 px-4 py-2.5">
      <p className="text-sm">
        <span className="font-medium">{t('rollback.title')}</span>{' '}
        <span className="font-mono text-xs">{label}</span>{' '}
        <span className="text-muted-foreground">
          {formatAge(prev.finished_at ?? prev.started_at)}
        </span>
      </p>
      <Button size="sm" variant="outline" onClick={() => setConfirming(true)}>
        {t('rollback.action')}
      </Button>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('rollback.confirmTitle', { name })}
        description={t('rollback.confirmBody')}
        confirmLabel={t('rollback.action')}
        pending={rollback.isPending}
        error={rollback.error?.message ?? null}
        onConfirm={() =>
          rollback.mutate(
            { deployId: prev.id, confirm: true },
            { onSuccess: () => setConfirming(false) },
          )
        }
      />
    </div>
  )
}
