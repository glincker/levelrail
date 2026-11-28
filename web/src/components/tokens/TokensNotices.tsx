import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '../kit/RelativeTime'
import { RevokeTokenDialog } from '../RevokeTokenDialog'
import { useUnusedTokens } from '../../queries/tokensUnused'
import type { TokenAbility } from '../../types/token'

// One compact notice for every token warning, so the page never stacks banners.
// expiringSoon is the server count of live tokens inside the warning window.
export function TokensNotices({
  expiringSoon,
  soonDays,
  longLivedRoot,
  onReviewExpiring,
}: {
  expiringSoon: number
  soonDays: number
  longLivedRoot: number
  onReviewExpiring: () => void
}) {
  const { t } = useTranslation('tokensNotices')
  const { t: tl } = useTranslation('tokenList')
  const unused = useUnusedTokens().data
  const rows = unused?.items ?? []
  const count =
    (expiringSoon > 0 ? 1 : 0) +
    (longLivedRoot > 0 ? 1 : 0) +
    (rows.length > 0 ? 1 : 0)
  if (count === 0) return null

  return (
    <Banner variant="warning" title={t('title', { count })}>
      <ul className="list-disc space-y-1 pl-4 break-words">
        {expiringSoon > 0 ? (
          <li>
            {tl('expiringBanner', { count: expiringSoon, days: soonDays })}{' '}
            <Button variant="link" size="sm" onClick={onReviewExpiring}>
              {tl('reviewExpiring')}
            </Button>
          </li>
        ) : null}
        {longLivedRoot > 0 ? (
          <li>{t('longLivedRoot', { count: longLivedRoot })}</li>
        ) : null}
        {rows.length > 0 ? (
          <li>
            {t('unusedIntro', { days: unused?.alert_days ?? 0 })}
            <ul className="mt-1 space-y-1">
              {rows.map((item) => (
                <li
                  key={item.id}
                  className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1"
                >
                  <span className="min-w-0 font-medium break-words">
                    {item.name}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {item.last_used_at ? (
                      <>
                        {t('lastUsed')} <RelativeTime at={item.last_used_at} />
                      </>
                    ) : (
                      t('neverUsed')
                    )}
                  </span>
                  <RevokeTokenDialog
                    token={{
                      ...item,
                      abilities: item.abilities as TokenAbility[],
                    }}
                  />
                </li>
              ))}
            </ul>
          </li>
        ) : null}
      </ul>
    </Banner>
  )
}
