import { toast } from '@/components/ui/toast'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import {
  useClearVolumeBackupSchedule,
  useSetVolumeBackupSchedule,
  useVolumeBackupSchedule,
} from '../queries/volumeBackupSchedule'
import {
  fromCron,
  scheduleRetentionSummary,
  toCron,
  type ScheduleFormValues,
} from '../lib/cronSchedule'
import { BackupScheduleFormView } from './BackupScheduleForm'

function toFieldValues(schedule: {
  target_id?: string
  schedule?: string
  retain?: number
  retain_days?: number
}): ScheduleFormValues {
  return {
    targetId: schedule.target_id ?? '',
    retain: String(schedule.retain ?? 7),
    retainDays: String(schedule.retain_days ?? 0),
    ...fromCron(schedule.schedule),
  }
}

// VolumeBackupScheduleForm is BackupScheduleForm's exact app service
// volume counterpart, sharing BackupScheduleFormView for the UI itself:
// wired against the volume's own dedicated GET/PUT/DELETE
// .../backup-schedule endpoint instead of the fields riding along on the
// app resource itself (see queries/volumeBackupSchedule.ts's own header
// comment for why that endpoint exists at all).
export function VolumeBackupScheduleForm({
  appName,
  volumeName,
}: {
  appName: string
  volumeName: string
}) {
  const targets = useBackupTargetsOptional().data ?? []
  const scheduleQuery = useVolumeBackupSchedule(appName, volumeName)
  const setSchedule = useSetVolumeBackupSchedule(appName, volumeName)
  const clearSchedule = useClearVolumeBackupSchedule(appName, volumeName)
  const schedule = scheduleQuery.data
  const scheduled = !!schedule?.schedule

  // No backup targets to schedule against yet, or the existing schedule
  // hasn't loaded: BackupScheduleFormView already hides itself once
  // targets is empty, but this form's default values come from an async
  // query rather than an already-loaded prop, so it must wait for that
  // query before handing values down.
  if (targets.length === 0 || scheduleQuery.isLoading) {
    return null
  }

  function handleSubmit(values: ScheduleFormValues) {
    setSchedule.mutate(
      {
        target_id: values.targetId,
        schedule: toCron(values),
        retain: Math.round(Number(values.retain)),
        retain_days: Math.round(Number(values.retainDays)),
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Backup schedule saved.', type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not save backup schedule.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleClear() {
    clearSchedule.mutate(undefined, {
      onSuccess: () => {
        toast.add({ title: 'Backup schedule removed.', type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not remove backup schedule.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const summary = scheduled
    ? `Runs on schedule "${schedule?.schedule}", ${scheduleRetentionSummary(schedule?.retain, schedule?.retain_days)}.`
    : 'No recurring backup configured for this volume.'

  return (
    <BackupScheduleFormView
      idPrefix="volume-backup-schedule"
      values={toFieldValues(schedule ?? {})}
      scheduled={scheduled}
      summary={summary}
      targets={targets}
      submitPending={setSchedule.isPending}
      submitError={setSchedule.isError ? setSchedule.error.message : null}
      clearPending={clearSchedule.isPending}
      onSubmit={handleSubmit}
      onClear={handleClear}
    />
  )
}
