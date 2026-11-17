import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { preflightQueryOptions, type UpdateStatus } from '@/queries/updates'
import { ReleaseNotes } from '../ReleaseNotes'
import { UpgradeNow } from '../UpgradeNow'
import { LazySection } from './LazySection'
import { UpdateMigrations } from './UpdateMigrations'

function Notes({ status }: { status: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const notes = preflight.data?.release_notes ?? ''
  const published = status.published_at ?? preflight.data?.release_published_at
  return (
    <>
      <p className="text-xs text-muted-foreground">
        {published
          ? t('quick.released', {
              date: new Date(published).toLocaleDateString(),
            })
          : t('changes.releasedUnknown')}
      </p>
      {notes !== '' ? (
        <div className="max-h-80 overflow-y-auto rounded-md border border-border bg-background p-3">
          <ReleaseNotes markdown={notes} />
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('changes.notesEmpty')}
        </p>
      )}
    </>
  )
}

export interface UpdateChangesProps {
  status: UpdateStatus
  showChanges: boolean
  skipBackup: boolean
  onSkipBackupChange: (v: boolean) => void
  onRequestUpgrade: () => void
  stalled: boolean
}

// Collapsed rows keep the first screen short; each loads its data only when
// opened.
export function UpdateChanges({
  status,
  showChanges,
  skipBackup,
  onSkipBackupChange,
  onRequestUpgrade,
  stalled,
}: UpdateChangesProps) {
  const { t } = useTranslation('updates')
  return (
    <div
      aria-label={t('changes.title')}
      role="group"
      className="divide-y divide-border rounded-lg border border-border"
    >
      {showChanges ? (
        <>
          <LazySection title={t('quick.rows.notes')}>
            <Notes status={status} />
          </LazySection>
          <LazySection title={t('quick.rows.migrations')}>
            <UpdateMigrations />
          </LazySection>
        </>
      ) : null}
      <LazySection title={t('quick.rows.plan')}>
        <UpgradeNow
          externalActions
          skipBackup={skipBackup}
          onSkipBackupChange={onSkipBackupChange}
          onRequestUpgrade={onRequestUpgrade}
          stalled={stalled}
        />
      </LazySection>
    </div>
  )
}
