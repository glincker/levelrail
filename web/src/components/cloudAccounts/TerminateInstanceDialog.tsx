import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { terminateName } from '../../lib/cloudInstances'
import type { CloudInstance } from '../../types/cloudAccounts'

export function TerminateInstanceDialog({
  instance,
  canKeepBootVolume,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: Readonly<{
  /** The dialog is open while an instance is set. */
  instance: CloudInstance | null
  canKeepBootVolume: boolean
  pending: boolean
  error: string | null
  onOpenChange: (open: boolean) => void
  onConfirm: (keepBootVolume: boolean) => void
}>) {
  const { t } = useTranslation('cloudAccounts')
  const [keep, setKeep] = useState(false)
  if (!instance) return null
  const name = terminateName(instance)
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) setKeep(false)
        onOpenChange(open)
      }}
      title={t('instances.terminate.title', { name })}
      description={t('instances.terminate.body')}
      confirmLabel={t('instances.terminate.confirm')}
      tone="destructive"
      requireTyped={name}
      pending={pending}
      error={error}
      onConfirm={() => {
        onConfirm(keep && canKeepBootVolume)
      }}
    >
      {canKeepBootVolume ? (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={keep}
            onCheckedChange={(v) => {
              setKeep(v === true)
            }}
          />
          {t('instances.terminate.keepVolume')}
        </label>
      ) : null}
    </ConfirmDialog>
  )
}
