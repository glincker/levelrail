import { useTranslation } from 'react-i18next'
import { MoonIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useSleepFleet } from '../../queries/sleepFleet'
import { formatSize } from '../../lib/format'

/** SleepFleetCard shows how many apps are asleep and the memory that frees up. */
export function SleepFleetCard() {
  const { t } = useTranslation('deploys')
  const { data } = useSleepFleet()
  if (!data) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <MoonIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          {t('appSleep.fleetTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {data.enabled === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('appSleep.fleetEmpty')}
          </p>
        ) : (
          <>
            <p className="text-sm font-medium text-foreground">
              {t('appSleep.fleetSummary', {
                sleeping: data.sleeping,
                enabled: data.enabled,
              })}
            </p>
            {data.reclaimed_bytes > 0 ? (
              <p className="text-xs text-muted-foreground">
                {t('appSleep.fleetReclaimed', {
                  size: formatSize(data.reclaimed_bytes),
                })}
              </p>
            ) : null}
            <ul className="space-y-1 pt-1">
              {data.apps
                .filter((a) => a.sleeping)
                .slice(0, 5)
                .map((a) => (
                  <li key={a.name} className="text-xs">
                    <Link
                      to="/apps/$name/deploys"
                      params={{ name: a.name }}
                      className="text-foreground hover:underline"
                    >
                      {a.name}
                    </Link>
                  </li>
                ))}
            </ul>
          </>
        )}
      </CardContent>
    </Card>
  )
}
