import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { preflightQueryOptions } from '../../queries/updates'
import { useRecheckPreflight } from '../../queries/upgradeSchedule'
import { Button } from '../ui/button'
import { CopyCommand } from './CopyCommand'
import { ReleaseNotes } from './ReleaseNotes'
import { UpgradeCheckRow } from './UpgradeCheckRow'
import { EdgeUpdateGuidance } from './EdgeUpdateGuidance'

export function UpgradePreflight() {
  const { t } = useTranslation('updates')
  const { data, isPending, isError } = useQuery(preflightQueryOptions())
  const recheck = useRecheckPreflight()
  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('preflight.running')}</p>
    )
  }
  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('preflight.unavailable')}
      </p>
    )
  }
  return (
    <div className="space-y-4">
      {data.release_notes ? (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">
            {t('notes.title')}
          </p>
          <div className="max-h-48 overflow-auto rounded-md bg-muted px-3 py-2">
            <ReleaseNotes markdown={data.release_notes} />
          </div>
        </div>
      ) : null}
      <ul className="space-y-3" aria-label={t('preflight.checksLabel')}>
        {data.checks.map((c) => (
          <UpgradeCheckRow
            key={c.code}
            check={c}
            cosignCommand={data.cosign_command}
          />
        ))}
      </ul>
      {data.waiting ? (
        <p className="text-sm text-muted-foreground">
          {t('preflight.waiting')}
        </p>
      ) : data.blocked ? (
        <p className="text-sm text-destructive">{t('preflight.blocked')}</p>
      ) : data.edge ? (
        <EdgeUpdateGuidance
          edge={data.edge}
          updateAvailable={data.update_available}
        />
      ) : data.update_available ? (
        <div className="space-y-3">
          <CopyCommand
            label={t('preflight.upgradeCommand')}
            command={data.upgrade_command}
          />
          <CopyCommand
            label={t('preflight.rollbackCommand')}
            command={data.rollback_command}
          />
        </div>
      ) : null}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={recheck.isPending}
        onClick={() => recheck.mutate()}
      >
        {recheck.isPending
          ? t('preflight.fix.rechecking')
          : t('preflight.recheckAll')}
      </Button>
    </div>
  )
}
