import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useProtectAll } from '../../queries/backupCoverage'
import { useStartBackupDrill } from '../../queries/backupProtection'
import { useTriggerBackup } from '../../queries/backupHistory'
import { useTriggerVolumeBackup } from '../../queries/volumeBackupHistory'
import type { CoverageRow, CoverageTarget } from '../../types/backupCoverage'
import { RestoreWizardDialog } from './RestoreWizardDialog'
import { VolumePolicyDialog } from './VolumePolicyDialog'

function ProtectButton({
  row,
  targets,
  onNeedTarget,
}: {
  row: CoverageRow
  targets: CoverageTarget[]
  onNeedTarget: () => void
}) {
  const { t } = useTranslation('backups')
  const protect = useProtectAll()
  return (
    <Button
      type="button"
      size="sm"
      disabled={protect.isPending}
      onClick={() => {
        const target = targets[0]
        if (!target) {
          onNeedTarget()
          return
        }
        protect.mutate(
          {
            target_id: target.id,
            apply: true,
            only: [{ kind: row.kind, app_name: row.app_name, name: row.name }],
          },
          {
            onSuccess: (res) => {
              const ok = (res.done?.length ?? 0) > 0
              toast.add({
                title: ok
                  ? t('row.protected', { name: row.name })
                  : t('row.protectFailed', { name: row.name }),
                type: ok ? 'success' : 'error',
              })
            },
            onError: (err) => {
              toast.add({
                title: t('row.protectFailed', { name: row.name }),
                description: err.message,
                type: 'error',
              })
            },
          },
        )
      }}
    >
      {t('row.protect')}
    </Button>
  )
}

function useBackupNowCallbacks(name: string) {
  const { t } = useTranslation('backups')
  return {
    onSuccess: () => {
      toast.add({ title: t('row.backupStarted', { name }), type: 'success' })
    },
    onError: (err: Error) => {
      toast.add({
        title: t('row.backupFailed', { name }),
        description: err.message,
        type: 'error',
      })
    },
  }
}

function DatabaseBackupNow({ row }: { row: CoverageRow }) {
  const { t } = useTranslation('backups')
  const trigger = useTriggerBackup(row.name)
  const callbacks = useBackupNowCallbacks(row.name)
  return (
    <Button
      type="button"
      size="sm"
      disabled={trigger.isPending || !row.target_id}
      onClick={() => {
        trigger.mutate(row.target_id ?? '', callbacks)
      }}
    >
      {t('row.backupNow')}
    </Button>
  )
}

function VolumeBackupNow({ row }: { row: CoverageRow }) {
  const { t } = useTranslation('backups')
  const trigger = useTriggerVolumeBackup(row.app_name ?? '', row.name)
  const callbacks = useBackupNowCallbacks(row.name)
  return (
    <Button
      type="button"
      size="sm"
      disabled={trigger.isPending || !row.target_id}
      onClick={() => {
        trigger.mutate(row.target_id ?? '', callbacks)
      }}
    >
      {t('row.backupNow')}
    </Button>
  )
}

function ProveRestoreButton({ row }: { row: CoverageRow }) {
  const { t } = useTranslation('backups')
  const drill = useStartBackupDrill()
  if (!row.last_backup_id) return null
  const backupId = row.last_backup_id
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={drill.isPending}
      onClick={() => {
        drill.mutate(backupId, {
          onSuccess: () => {
            toast.add({
              title: t('health.drillStarted'),
              description: t('health.drillStartedDescription'),
              type: 'success',
            })
          },
          onError: (err) => {
            toast.add({
              title: t('health.drillFailedToast'),
              description: err.message,
              type: 'error',
            })
          },
        })
      }}
    >
      {t('health.runDrill')}
    </Button>
  )
}

/** CoverageRowActions is the primary action of a coverage row plus the proof-of-restore shortcut. */
export function CoverageRowActions({
  row,
  targets,
  onNeedTarget,
}: {
  row: CoverageRow
  targets: CoverageTarget[]
  onNeedTarget: () => void
}) {
  const { t } = useTranslation('backups')
  const [restoreOpen, setRestoreOpen] = useState(false)
  const [policyOpen, setPolicyOpen] = useState(false)
  const isVolume = row.kind === 'volume'

  return (
    <div className="flex flex-wrap items-center gap-2">
      {row.primary_action === 'protect' ? (
        <ProtectButton
          row={row}
          targets={targets}
          onNeedTarget={onNeedTarget}
        />
      ) : null}
      {row.primary_action === 'backup_now' ? (
        isVolume ? (
          <VolumeBackupNow row={row} />
        ) : (
          <DatabaseBackupNow row={row} />
        )
      ) : null}
      {row.primary_action === 'restore' ? (
        isVolume ? (
          <>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setRestoreOpen(true)
              }}
            >
              {t('health.restore')}
            </Button>
            <RestoreWizardDialog
              app={row.app_name ?? ''}
              volume={row.name}
              open={restoreOpen}
              onOpenChange={setRestoreOpen}
            />
          </>
        ) : (
          <Button
            size="sm"
            variant="outline"
            render={
              <Link
                to="/databases/$name/overview"
                params={{ name: row.name }}
              />
            }
            nativeButton={false}
          >
            {t('health.restoreDatabase')}
          </Button>
        )
      ) : null}
      {row.state === 'unverified' ? <ProveRestoreButton row={row} /> : null}
      {isVolume ? (
        <>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => {
              setPolicyOpen(true)
            }}
          >
            {t('health.policy')}
          </Button>
          <VolumePolicyDialog
            app={row.app_name ?? ''}
            volume={row.name}
            open={policyOpen}
            onOpenChange={setPolicyOpen}
          />
        </>
      ) : null}
    </div>
  )
}
