import { useTranslation } from 'react-i18next'
import type { UpdatePreflight } from '../../queries/updates'

// UpgradePlanPreview states what one Upgrade click does before it is pressed.
export function UpgradePlanPreview({ pre }: { pre: UpdatePreflight }) {
  const { t } = useTranslation('updates')
  if (pre.state !== 'upgrade_available' || !pre.latest_version) return null
  const migrations = pre.migrations_to_apply
  return (
    <section aria-label={t('upgradeNow.hero.planTitle')} className="space-y-1">
      <p className="text-sm font-medium">
        {t('upgradeNow.hero.planFromTo', {
          from: pre.current_version,
          to: pre.latest_version,
        })}
      </p>
      <ul className="list-disc space-y-0.5 pl-5 text-sm text-muted-foreground">
        <li>
          {typeof migrations === 'number'
            ? t('upgradeNow.hero.planMigrations', { count: migrations })
            : t('upgradeNow.hero.planMigrationsUnknown')}
        </li>
        {pre.restore_point_first ? (
          <li>{t('upgradeNow.hero.planRestorePoint')}</li>
        ) : null}
        {pre.verify_window_seconds ? (
          <li>
            {t('upgradeNow.hero.planVerify', {
              seconds: pre.verify_window_seconds,
            })}
          </li>
        ) : null}
      </ul>
    </section>
  )
}
