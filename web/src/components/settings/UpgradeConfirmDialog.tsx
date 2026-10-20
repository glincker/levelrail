import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { Checkbox } from '../ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog'
import type { SelfUpgradePlan } from '../../queries/selfUpgrade'

export function UpgradeConfirmDialog({
  open,
  onOpenChange,
  plan,
  pending,
  error,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: SelfUpgradePlan
  pending: boolean
  error?: string
  onConfirm: (ack: string[]) => void
}) {
  const { t } = useTranslation('updates')
  const [acked, setAcked] = useState<Record<string, boolean>>({})
  const needAck = plan.breaking.filter((b) => b.requires_ack)
  const allAcked = needAck.every((b) => acked[b.id])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('upgradeNow.confirmTitle', { version: plan.target_version })}
          </DialogTitle>
          <DialogDescription>
            {t('upgradeNow.confirmDescription', {
              current: plan.current_version,
              target: plan.target_version,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          {plan.breaking.length > 0 ? (
            <div className="space-y-2">
              <p className="font-medium">{t('selfUpgrade.breakingTitle')}</p>
              <ul className="space-y-2">
                {plan.breaking.map((b) => (
                  <li
                    key={b.id}
                    className="flex items-start gap-2 rounded-md bg-muted px-3 py-2"
                  >
                    {b.requires_ack ? (
                      <Checkbox
                        id={`ack-${b.id}`}
                        checked={acked[b.id] === true}
                        onCheckedChange={(v) =>
                          setAcked((prev) => ({ ...prev, [b.id]: v === true }))
                        }
                      />
                    ) : null}
                    <label htmlFor={`ack-${b.id}`} className="min-w-0 flex-1">
                      <span className="font-mono text-xs text-muted-foreground">
                        {b.version}
                      </span>{' '}
                      {b.summary}
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="text-muted-foreground">
              {t('selfUpgrade.noBreaking')}
            </p>
          )}
          <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
            <li>{t('upgradeNow.willBackup')}</li>
            <li>{t('upgradeNow.willRestart')}</li>
            <li>{t('upgradeNow.willRollback')}</li>
          </ul>
          {error ? (
            <p role="alert" className="text-destructive">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('schedule.cancel')}
          </Button>
          <Button
            type="button"
            disabled={pending || !allAcked}
            onClick={() =>
              onConfirm(needAck.filter((b) => acked[b.id]).map((b) => b.id))
            }
          >
            {pending ? t('selfUpgrade.starting') : t('upgradeNow.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
