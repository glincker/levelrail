import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { useTriggerBackup } from '../../queries/backupHistory'
import {
  useClearActionParam,
  type DatabasePaletteAction,
} from '../../lib/paletteAction'
import { engineHasBackups } from '@/lib/databaseCatalog'
import type { DatabaseResource } from '../../types/databaseDetail'

/** Opens the page's own confirm step for a ?action=backup link from the command palette. */
export function DatabasePaletteActions({
  database,
  action,
}: {
  database: DatabaseResource
  action: DatabasePaletteAction | undefined
}) {
  const { t } = useTranslation('databases')
  const clear = useClearActionParam()
  const [open, setOpen] = useState(false)
  const backup = useTriggerBackup(database.name)
  const targetId = database.backup_target_id
  const canBackUp =
    !database.suspended &&
    engineHasBackups(database.engine) &&
    Boolean(targetId)

  const [handled, setHandled] = useState<DatabasePaletteAction>()
  if (action !== handled) {
    setHandled(action)
    if (action === 'backup' && canBackUp) setOpen(true)
  }

  useEffect(() => {
    if (action) clear()
  }, [action, clear])

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={setOpen}
      title={t('detail.backupConfirmTitle', { name: database.name })}
      description={t('detail.backupConfirmDescription')}
      consequences={[t('detail.backupConfirmConsequence')]}
      confirmLabel={t('detail.backupNow')}
      pendingLabel={t('detail.backupStarting')}
      pending={backup.isPending}
      onConfirm={() => {
        if (!targetId) return
        backup.mutate(targetId, {
          onSuccess: () => {
            toast.add({
              title: t('detail.backupStarted', { name: database.name }),
              type: 'success',
            })
            setOpen(false)
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    />
  )
}
