import type { TFunction } from 'i18next'
import { formatBytes } from '../../lib/format'
import { formatRelativeTime } from '../../lib/relativeTime'
import type { BackupDrill } from '../../types/backupProtection'

export function restoreTestSummary(
  t: TFunction<'backups'>,
  drill: BackupDrill | undefined,
): string {
  if (!drill) return t('restoreTest.neverRun')
  if (drill.status === 'running') return t('restoreTest.running')
  const when = formatRelativeTime(drill.finished_at ?? drill.started_at)
  const seconds = Math.max(1, Math.round(drill.duration_ms / 1000))
  if (drill.status === 'passed') {
    return t('restoreTest.ok', {
      when,
      seconds,
      size: formatBytes(drill.bytes),
    })
  }
  return t('restoreTest.failed', { when, seconds })
}
