import { useTranslation } from 'react-i18next'
import { useBlocker } from '@tanstack/react-router'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'

export function UnsavedGuardDialog({
  isDirty,
}: Readonly<{ isDirty: boolean }>) {
  const { t } = useTranslation('common')
  const blocker = useBlocker({
    shouldBlockFn: () => true,
    disabled: !isDirty,
    enableBeforeUnload: false,
    withResolver: true,
  })
  return (
    <ConfirmDialog
      open={blocker.status === 'blocked'}
      onOpenChange={(open) => {
        if (!open && blocker.status === 'blocked') blocker.reset()
      }}
      title={t('unsavedGuard.title')}
      description={t('unsavedGuard.description')}
      confirmLabel={t('unsavedGuard.discard')}
      cancelLabel={t('unsavedGuard.keep')}
      tone="destructive"
      onConfirm={() => {
        if (blocker.status === 'blocked') blocker.proceed()
      }}
    />
  )
}
