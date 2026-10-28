import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { writeSkipRestartConfirm, type PowerKind } from './appPower'

/** One compact confirm for app Restart and Stop; only restart can be muted. */
export function AppPowerConfirm({
  kind,
  name,
  pending,
  onCancel,
  onConfirm,
}: {
  kind: PowerKind | null
  name: string
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  const { t } = useTranslation('common')
  const [skip, setSkip] = useState(false)
  const stop = kind === 'stop'
  return (
    <ConfirmDialog
      open={kind !== null}
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
      title={t(stop ? 'appPower.stopTitle' : 'appPower.restartTitle', { name })}
      description={t(
        stop ? 'appPower.stopDescription' : 'appPower.restartDescription',
      )}
      consequences={
        stop
          ? [
              t('appPower.stopConsequenceRequests'),
              t('appPower.stopConsequencePolicy'),
            ]
          : [
              t('appPower.restartConsequenceRequests'),
              t('appPower.restartConsequencePolicy'),
            ]
      }
      confirmLabel={t(
        stop ? 'appPower.stopConfirm' : 'appPower.restartConfirm',
      )}
      pendingLabel={t('appPower.pending')}
      tone={stop ? 'destructive' : 'default'}
      pending={pending}
      onConfirm={() => {
        if (!stop) writeSkipRestartConfirm(skip)
        onConfirm()
      }}
    >
      {stop ? null : (
        <div className="flex items-center gap-2">
          <Checkbox
            id="app-power-skip"
            checked={skip}
            onCheckedChange={(next) => {
              setSkip(next === true)
            }}
          />
          <Label htmlFor="app-power-skip" className="text-sm font-normal">
            {t('appPower.restartSkip')}
          </Label>
        </div>
      )}
    </ConfirmDialog>
  )
}
