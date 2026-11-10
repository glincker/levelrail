import type { PlatformImportReport } from '../queries/platformImport'

export interface ExtrasToggles {
  includeCredentials: boolean
  enableSchedules: boolean
  enableChannels: boolean
}

export const defaultExtrasToggles: ExtrasToggles = {
  includeCredentials: false,
  enableSchedules: false,
  enableChannels: false,
}

export function hasExtras(report: PlatformImportReport): boolean {
  return (
    (report.backups?.targets.length ?? 0) +
      (report.backups?.schedules.length ?? 0) +
      (report.channels?.channels.length ?? 0) +
      (report.channels?.unsupported?.length ?? 0) >
    0
  )
}
