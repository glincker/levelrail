import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { ROLLBACK_SECTION_ID } from '../../lib/updatesAnchors'
import type { UpdatePreflight } from '../../queries/updates'

interface HeroProps {
  pre: UpdatePreflight
  checking: boolean
  onCheckAgain: () => void
}

// UpgradeStateHero explains an install whose data is newer than every
// published release, instead of listing the releases it cannot accept.
export function UpgradeStateHero({ pre, checking, onCheckAgain }: HeroProps) {
  const { t } = useTranslation('updates')
  const latest = pre.latest_version ?? ''
  const schema = pre.db_schema
  const rollback = pre.rollback_version
  const protects = pre.checks.find((c) => c.code === 'schema_compat')?.protects
  return (
    <div className="space-y-3" data-state="ahead_of_releases">
      <p className="text-sm font-medium">{t('upgradeNow.hero.aheadTitle')}</p>
      <p className="text-sm text-muted-foreground">
        {typeof schema === 'number'
          ? t('upgradeNow.hero.aheadBody', { schema, latest })
          : t('upgradeNow.hero.aheadBodyNoSchema', { latest })}
      </p>
      {protects ? (
        <p className="text-xs text-muted-foreground">
          {t('preflight.protects', { what: protects })}
        </p>
      ) : null}
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Button
          type="button"
          variant="outline"
          disabled={checking}
          onClick={onCheckAgain}
        >
          {checking
            ? t('upgradeNow.hero.checking')
            : t('upgradeNow.hero.checkAgain')}
        </Button>
        {pre.release_url ? (
          <Button
            variant="outline"
            render={
              <a href={pre.release_url} target="_blank" rel="noreferrer" />
            }
          >
            {t('upgradeNow.hero.notes')}
          </Button>
        ) : null}
        {rollback ? (
          <Button
            type="button"
            variant="outline"
            onClick={() =>
              document
                .getElementById(ROLLBACK_SECTION_ID)
                ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
            }
          >
            {t('upgradeNow.hero.rollback', { version: rollback })}
          </Button>
        ) : (
          <>
            <Button
              type="button"
              variant="outline"
              disabled
              aria-describedby="upgrade-rollback-why"
            >
              {t('upgradeNow.hero.rollbackNone')}
            </Button>
            <span
              id="upgrade-rollback-why"
              className="text-xs text-muted-foreground"
            >
              {t('upgradeNow.hero.rollbackNoneWhy')}
            </span>
          </>
        )}
      </div>
    </div>
  )
}
