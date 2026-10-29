import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { useDatabaseUpgrades } from '../../queries/databaseUpgrades'
import type { UpgradeKind } from '../../types/databaseUpgrades'

const STEP_KEYS = ['backup', 'change', 'verify'] as const

export function UpgradeStepList({
  databaseName,
  kind,
}: {
  databaseName: string
  kind?: UpgradeKind
}) {
  const { t } = useTranslation('databaseUpgrades')
  const policy = useDatabaseUpgrades(databaseName).data?.policy
  const backupFirst = policy?.backup_before ?? true
  const verify = policy?.verify_after ?? true
  const autoRevert = policy?.revert_on_failure ?? false

  const skipped: Record<(typeof STEP_KEYS)[number], boolean> = {
    backup: !backupFirst,
    change: false,
    verify: !verify,
  }

  return (
    <div className="space-y-3">
      {kind === 'major' ? (
        <p
          className="flex items-start gap-2 rounded-md border border-destructive/40 p-3 text-sm text-destructive"
          role="alert"
        >
          <TriangleAlertGlassIcon
            className="mt-0.5 size-4.5 shrink-0 text-tone-warning"
            aria-hidden="true"
          />
          {t('steps.majorWarning')}
        </p>
      ) : null}
      <ol className="space-y-2">
        {STEP_KEYS.map((key, i) => (
          <li key={key} className="flex gap-3 text-sm">
            <span className="flex size-5 shrink-0 items-center justify-center rounded-full border border-border text-xs">
              {i + 1}
            </span>
            <span className="min-w-0">
              <span className="font-medium text-foreground">
                {t(`steps.${key}.title`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {skipped[key]
                  ? t(`steps.${key}.skipped`)
                  : t(`steps.${key}.body`)}
              </span>
            </span>
          </li>
        ))}
      </ol>
      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <RotateCcwGlassIcon
          className="mt-0.5 size-4.5 shrink-0"
          aria-hidden="true"
        />
        {autoRevert ? t('steps.rollbackAuto') : t('steps.rollbackManual')}
      </p>
    </div>
  )
}
