import { useTranslation } from 'react-i18next'
import type { SelfUpgradeFailure } from '@/queries/selfUpgrade'

// Surfaces the server's failure diagnosis next to the action bar.
export function UpgradeFailureNote({
  failure,
}: {
  failure: SelfUpgradeFailure
}) {
  const { t } = useTranslation('updates')
  const cause = t(`selfUpgrade.failure.cause.${failure.cause}`, {
    defaultValue: t('selfUpgrade.failure.cause.unknown'),
  })
  return (
    <div
      role="alert"
      className="space-y-1 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive"
    >
      <p>{t('control.failedCause', { cause })}</p>
      {failure.cli ? (
        <p className="font-mono text-xs break-all">
          {t('control.failedCli', { cli: failure.cli })}
        </p>
      ) : null}
    </div>
  )
}
