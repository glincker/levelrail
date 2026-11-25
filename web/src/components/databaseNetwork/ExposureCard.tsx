import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { DatabaseNetwork } from '../../types/databaseAccess'

type Level = 'private' | 'loopback' | 'public'

const LOOPBACK = new Set(['127.0.0.1', '::1', 'localhost'])

// Derived only from the published port data the API already returns.
function currentLevel(network: DatabaseNetwork): Level {
  const bind = network.published?.bind
  if (!network.published) return 'private'
  if (bind && bind.length > 0 && bind.every((b) => LOOPBACK.has(b))) {
    return 'loopback'
  }
  return 'public'
}

/** ExposureCard states who can reach the database; selecting a level arrives with the rules backend, so it is read only here. */
export function ExposureCard({ network }: { network: DatabaseNetwork }) {
  const { t } = useTranslation('dbNav')
  const current = currentLevel(network)
  const canPublic = network.rules.can_restrict
  const levels: Level[] = ['private', 'loopback', 'public']

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <GlobeGlassIcon className="size-5" aria-hidden="true" />
          {t('networking.exposureTitle')}
        </CardTitle>
        <p className="text-xs text-muted-foreground">
          {t('networking.exposureDescription')}
        </p>
      </CardHeader>
      <CardContent className="space-y-2">
        <ul className="grid gap-2 sm:grid-cols-3">
          {levels.map((level) => {
            const unavailable = level === 'public' && !canPublic
            return (
              <li
                key={level}
                aria-disabled={unavailable || undefined}
                className="flex flex-col gap-0.5 rounded-md border p-2 text-sm aria-disabled:opacity-60"
              >
                <span className="flex items-center gap-1.5 font-medium">
                  {t(`networking.${level}`)}
                  {current === level ? (
                    <Badge variant="muted">{t('networking.current')}</Badge>
                  ) : null}
                  {unavailable ? (
                    <Badge variant="muted">{t('networking.unavailable')}</Badge>
                  ) : null}
                </span>
                <span className="text-xs text-muted-foreground">
                  {t(`networking.${level}Hint`)}
                </span>
              </li>
            )
          })}
        </ul>
        {!canPublic ? (
          <p className="text-xs text-muted-foreground">
            {t('networking.publicDisabled', {
              reason: network.rules.cannot_restrict_reason ?? '',
            })}
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t('networking.changeNote')}
        </p>
      </CardContent>
    </Card>
  )
}
