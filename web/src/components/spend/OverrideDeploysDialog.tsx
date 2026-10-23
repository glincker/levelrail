import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useOverrideSpendGuard } from '../../queries/spendCap'

/** OverrideDeploysDialog lets a root user allow deploys for a short, audited window. */
export function OverrideDeploysDialog({
  appName,
  open,
  onOpenChange,
}: {
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('budget')
  const override = useOverrideSpendGuard(appName)
  const [reason, setReason] = useState('')
  const id = useId()
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) override.reset()
        onOpenChange(next)
      }}
      title={t('override.title')}
      description={t('override.description')}
      consequences={[t('override.audited'), t('override.expires')]}
      confirmLabel={t('override.confirm')}
      pendingLabel={t('override.pending')}
      pending={override.isPending}
      error={override.error?.message ?? null}
      onConfirm={() => {
        if (reason.trim() === '') return
        override.mutate(reason.trim(), {
          onSuccess: () => {
            setReason('')
            onOpenChange(false)
            toast.add({ title: t('toast.overridden'), type: 'success' })
          },
        })
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor={id}>{t('override.reason')}</Label>
        <Input
          id={id}
          value={reason}
          maxLength={500}
          onChange={(e) => setReason(e.target.value)}
          placeholder={t('override.reasonPlaceholder')}
        />
      </div>
    </ConfirmDialog>
  )
}
