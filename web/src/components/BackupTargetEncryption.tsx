import { useTranslation } from 'react-i18next'
import { LockKeyIcon, LockSimpleOpenIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { toast } from '@/components/ui/toast'
import { useSetBackupTargetEncryption } from '../queries/backupTargetEncryption'
import type { BackupTarget } from '../types/backupTarget'

export function DumpEncryptionBadge({ target }: { target: BackupTarget }) {
  const { t } = useTranslation('settings')
  return target.encrypt_dumps ? (
    <Badge variant="success" className="gap-1.5">
      <LockKeyIcon className="size-3.5" aria-hidden="true" />
      {t('backupTargetsPage.encryption.on')}
    </Badge>
  ) : (
    <Badge variant="muted" className="gap-1.5">
      <LockSimpleOpenIcon className="size-3.5" aria-hidden="true" />
      {t('backupTargetsPage.encryption.off')}
    </Badge>
  )
}

// Root only on the server; a non-root caller sees the API's own error in the dialog.
export function EnableDumpEncryptionButton({
  target,
}: {
  target: BackupTarget
}) {
  const { t } = useTranslation('settings')
  const setEncryption = useSetBackupTargetEncryption()
  if (target.encrypt_dumps) return null

  return (
    <ConfirmAction
      trigger={(openDialog) => (
        <Button type="button" variant="outline" size="sm" onClick={openDialog}>
          <LockKeyIcon className="size-3.5" aria-hidden="true" />
          {t('backupTargetsPage.encryption.enable')}
        </Button>
      )}
      title={t('backupTargetsPage.encryption.title', { name: target.name })}
      consequences={[
        t('backupTargetsPage.encryption.consequenceNew'),
        t('backupTargetsPage.encryption.consequenceOld'),
        t('backupTargetsPage.encryption.consequenceKey'),
      ]}
      confirmLabel={t('backupTargetsPage.encryption.confirm')}
      pendingLabel={t('backupTargetsPage.encryption.pending')}
      pending={setEncryption.isPending}
      error={setEncryption.isError ? setEncryption.error.message : null}
      onOpenChange={(next) => {
        if (!next) setEncryption.reset()
      }}
      onConfirm={(close) => {
        setEncryption.mutate(
          { id: target.id, enabled: true },
          {
            onSuccess: () => {
              close()
              toast.add({
                title: t('backupTargetsPage.encryption.enabled', {
                  name: target.name,
                }),
                type: 'success',
              })
            },
          },
        )
      }}
    />
  )
}
