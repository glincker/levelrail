import type { z } from 'zod'
import type { BackupResourceKind, CreateAlertRuleRequest } from '../types/alerts'

// Same sanity-check regex CreateAlertRuleDialog/EditAlertRuleDialog each
// keep their own copy of for their other kinds' own duration fields: not
// a real time.Duration parser, just catches an obviously wrong value
// before a round trip.
const GO_DURATION_REGEX = /^-?(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

export interface BackupMissingFormShape {
  backupResourceKind: BackupResourceKind | ''
  backupDatabaseName: string
  backupVolumeName: string
  forDuration: string
}

// addBackupMissingIssues is the backup_missing branch of both
// CreateAlertRuleDialog's and EditAlertRuleDialog's zod
// .superRefine(), factored out since the two schemas otherwise repeat
// it verbatim.
export function addBackupMissingIssues(data: BackupMissingFormShape, ctx: z.RefinementCtx) {
  if (!data.backupResourceKind) {
    ctx.addIssue({ code: 'custom', message: 'Choose what this rule watches', path: ['backupResourceKind'] })
  } else if (data.backupResourceKind === 'database' && !data.backupDatabaseName) {
    ctx.addIssue({ code: 'custom', message: 'Choose which database to watch', path: ['backupDatabaseName'] })
  } else if (data.backupResourceKind === 'volume' && !data.backupVolumeName) {
    ctx.addIssue({ code: 'custom', message: 'Choose which volume to watch', path: ['backupVolumeName'] })
  }
  if (data.forDuration && !GO_DURATION_REGEX.test(data.forDuration)) {
    ctx.addIssue({ code: 'custom', message: 'Must look like a duration, e.g. "6h"', path: ['forDuration'] })
  }
}

// applyBackupMissingSubmitFields is the backup_missing branch of both
// dialogs' onSubmit request-building, factored out for the same reason.
export function applyBackupMissingSubmitFields(
  req: CreateAlertRuleRequest,
  values: BackupMissingFormShape,
  appName: string,
) {
  req.backup_resource_kind = values.backupResourceKind || undefined
  req.for_duration = values.forDuration.trim() || undefined
  if (values.backupResourceKind === 'database') {
    req.backup_database_name = values.backupDatabaseName
  } else if (values.backupResourceKind === 'volume') {
    // A volume's owning service is always this same app in this
    // codebase's single-service-per-app model, so backup_service_name is
    // set here rather than exposed as its own form field.
    req.backup_service_name = appName
    req.backup_volume_name = values.backupVolumeName
  }
}
