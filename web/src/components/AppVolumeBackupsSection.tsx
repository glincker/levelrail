import { useMemo, useState } from 'react'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import {
  VOLUME_BACKUP_HISTORY_PAGE_SIZE,
  fetchVolumeBackupHistory,
  useTriggerVolumeBackup,
  useVolumeBackupHistory,
  volumeBackupDownloadURL,
} from '../queries/volumeBackupHistory'
import { useVolumeRestoreHistory } from '../queries/volumeRestoreHistory'
import { RestoreVolumeBackupDialog } from './RestoreVolumeBackupDialog'
import { VolumeCloneRestoreDialog } from './VolumeCloneRestoreDialog'
import { VolumeCloneRestoreHistoryTable } from './VolumeCloneRestoreHistoryTable'
import { VolumeBackupScheduleForm } from './VolumeBackupScheduleForm'
import { VolumeBackupVerificationBadge } from './VolumeBackupVerificationBadge'
import { RestoreHistoryTableView } from './RestoreHistoryTable'
import {
  BackupHistoryTableView,
  DownloadBackupLinkView,
  TriggerBackupRowView,
  useBackupHistoryPagination,
} from './BackupsSection'
import type { AppVolume } from '../types/appDetail'

// Trigger-and-history section for one app's named Docker volumes, the
// direct app-volume counterpart of BackupsSection (which does this for a
// managed database's own volume, implicitly, one per database). An app
// can declare any number of volumes in app.yaml, so this section adds a
// volume picker BackupsSection never needed, and every sub-component
// below takes an explicit volumeName instead of assuming there's only
// one.
//
// VolumeCloneRestoreDialog's "restore as new volume" creates a bare,
// standalone Docker volume with no app or app.yaml attached to it and no
// detail page anywhere in this app: an operator can see it happened here,
// in VolumeCloneRestoreHistoryTable, and reference the new volume by name
// elsewhere, but attaching it to a running app is a manual step today.
//
// See queries/volumeBackupHistory.ts, queries/volumeRestoreHistory.ts,
// queries/volumeBackupVerification.ts, queries/volumeBackupSchedule.ts
// for the wire layer this composes, and BackupsSection's own header
// comment for the reasoning this mirrors in full (non-suspense queries,
// self-terminating polling while an attempt is running, the schedule
// form's daily/weekly/custom-cron shape via lib/cronSchedule.ts).

function TriggerVolumeBackupRow({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const triggerBackup = useTriggerVolumeBackup(appName, volumeName)
  return (
    <TriggerBackupRowView
      pickerId="volume-backup-target-picker"
      trigger={triggerBackup}
    />
  )
}

function VolumeBackupHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const targetsQuery = useBackupTargetsOptional()
  const { data, isLoading, error } = useVolumeBackupHistory(appName, volumeName)
  const firstPage = useMemo(() => data ?? [], [data])
  const pagination = useBackupHistoryPagination(
    firstPage,
    (before) => fetchVolumeBackupHistory(appName, volumeName, { before }),
    VOLUME_BACKUP_HISTORY_PAGE_SIZE,
  )

  const targetName = useMemo(() => {
    const targets = targetsQuery.data ?? []
    const byId = new Map(targets.map((t) => [t.id, t.name]))
    return (targetId: string) => byId.get(targetId) ?? 'Deleted target'
  }, [targetsQuery.data])

  return (
    <BackupHistoryTableView
      isLoading={isLoading}
      error={error}
      history={pagination.history}
      emptyMessage="No backups triggered yet for this volume."
      exhausted={pagination.exhausted}
      loadingMore={pagination.loadingMore}
      loadMoreError={pagination.loadMoreError}
      onLoadMore={() => {
        void pagination.handleLoadMore()
      }}
      targetName={targetName}
      renderVerification={(record) => (
        <VolumeBackupVerificationBadge
          appName={appName}
          volumeName={volumeName}
          backup={record}
        />
      )}
      renderActions={(record) => (
        <>
          <DownloadBackupLinkView
            href={volumeBackupDownloadURL(appName, volumeName, record.id)}
          />
          <RestoreVolumeBackupDialog
            appName={appName}
            volumeName={volumeName}
            backup={record}
          />
          <VolumeCloneRestoreDialog
            appName={appName}
            volumeName={volumeName}
            backup={record}
          />
        </>
      )}
    />
  )
}

// VolumeRestoreHistoryTable mirrors RestoreHistoryTable's exact shape via
// the shared RestoreHistoryTableView, only the data source differs.
function VolumeRestoreHistoryTable({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const { data, isLoading, error } = useVolumeRestoreHistory(
    appName,
    volumeName,
  )
  return (
    <RestoreHistoryTableView
      history={data ?? []}
      isLoading={isLoading}
      error={error}
    />
  )
}

function NoVolumesDeclared() {
  return (
    <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border px-6 py-8 text-center">
      <HardDrivesIcon
        className="size-5 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-sm text-muted-foreground">
        This app has no named volumes declared in app.yaml.
      </p>
    </div>
  )
}

export function AppVolumeBackupsSection({
  appName,
  volumes,
}: {
  appName: string
  volumes: AppVolume[] | undefined
}) {
  const [selected, setSelected] = useState<string | undefined>(
    volumes?.[0]?.name,
  )
  const volumeName = selected ?? volumes?.[0]?.name

  if (!volumes || volumes.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Volume backups</CardTitle>
        </CardHeader>
        <CardContent>
          <NoVolumesDeclared />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Volume backups</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {volumes.length > 1 ? (
          <Field className="w-full sm:w-64">
            <FieldLabel htmlFor="volume-picker">Volume</FieldLabel>
            <Select
              value={volumeName}
              onValueChange={(value: string | null) => {
                if (value) setSelected(value)
              }}
            >
              <SelectTrigger id="volume-picker" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {volumes.map((v) => (
                  <SelectItem key={v.name} value={v.name}>
                    {v.name} ({v.container_path})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        ) : null}
        {volumeName ? (
          <div className="space-y-4" key={volumeName}>
            <VolumeBackupScheduleForm
              appName={appName}
              volumeName={volumeName}
            />
            <TriggerVolumeBackupRow appName={appName} volumeName={volumeName} />
            <VolumeBackupHistoryTable
              appName={appName}
              volumeName={volumeName}
            />
            <VolumeRestoreHistoryTable
              appName={appName}
              volumeName={volumeName}
            />
            <VolumeCloneRestoreHistoryTable
              appName={appName}
              volumeName={volumeName}
            />
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
