import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useSetBackupPinned } from '../queries/backupHistory'
import type { BackupHistoryRecord } from '../types/backupHistory'

// Row action that pins one succeeded backup so retention and delete never
// remove it, or releases the pin again.
export function PinBackupButton({ record }: { record: BackupHistoryRecord }) {
  const { t } = useTranslation('backupPins')
  const setPinned = useSetBackupPinned()
  const pinned = record.pinned === true
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={setPinned.isPending}
      aria-pressed={pinned}
      title={pinned ? t('unpinHint') : t('pinHint')}
      onClick={() =>
        setPinned.mutate(
          { record, pinned: !pinned },
          {
            onSuccess: () =>
              toast.add({
                title: pinned ? t('unpinnedToast') : t('pinnedToast'),
                type: 'success',
              }),
            onError: (error) =>
              toast.add({
                title: pinned ? t('unpinFailed') : t('pinFailed'),
                description: error.message,
                type: 'error',
              }),
          },
        )
      }
    >
      <LockGlassIcon size="1.2em" aria-hidden="true" />
      {pinned ? t('unpin') : t('pin')}
    </Button>
  )
}
