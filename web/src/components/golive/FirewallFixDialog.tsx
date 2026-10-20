import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import type { FixPreview } from '../../queries/goLiveChecklist'

/** FirewallFixDialog shows exactly what will run, how to undo it and whether it survives a reboot, before anything changes. */
export function FirewallFixDialog({
  fix,
  open,
  pending,
  error,
  onOpenChange,
  onConfirm,
}: {
  fix: FixPreview
  open: boolean
  pending: boolean
  error: string | null
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation('golive')
  const consequences = [
    ...fix.commands.map((c) => t('fix.willRun', { command: c })),
    ...fix.undo.map((c) => t('fix.undo', { command: c })),
  ]
  if (!fix.persistent) consequences.push(t('fix.notPersistent'))
  if (fix.note && fix.persistent) consequences.push(fix.note)
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('fix.title')}
      description={t('fix.description')}
      consequences={consequences}
      confirmLabel={t('fix.confirm')}
      pendingLabel={t('fix.pending')}
      pending={pending}
      error={error}
      onConfirm={onConfirm}
    />
  )
}
