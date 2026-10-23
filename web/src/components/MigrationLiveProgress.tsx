import { useTranslation } from 'react-i18next'
import { Progress } from '@/components/ui/progress'
import { useMigrationJobEvents } from '../queries/migrationJob'
import type { AppImportSession } from '../queries/appImport'
import { isStaged } from './appImportFormat'

// Each event refetches the session, so the counts below are live while a run is active and
// still right after a reload.
export function MigrationLiveProgress({
  session,
  verifyView,
  active,
}: {
  session: AppImportSession
  verifyView: boolean
  active: boolean
}) {
  const { t } = useTranslation('migration')
  useMigrationJobEvents(session.id, active)
  const selected = session.items.filter((i) => i.selected)
  const total = selected.length
  if (total === 0) return null
  const failed = selected.filter(
    (i) => i.state === 'stage-failed' || i.state === 'verify-failed',
  ).length
  const done = selected.filter((i) =>
    verifyView
      ? i.state === 'verified' || i.state === 'routed'
      : isStaged(i.state),
  ).length
  const percent = Math.round((done / total) * 100)
  const label = t('appImport.run.progressTitle')
  return (
    <div className="space-y-1.5" role="group" aria-label={label}>
      <Progress value={percent} aria-label={label} className="gap-1" />
      <p
        role="status"
        aria-live="polite"
        className="text-xs text-muted-foreground"
      >
        {t('appImport.run.progress', { done, total, failed })}
      </p>
      {failed > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('appImport.run.failedHelp')}
        </p>
      ) : null}
    </div>
  )
}
