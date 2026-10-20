import { useTranslation } from 'react-i18next'
import { InfoIcon, ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import type { DatabaseUpgrades } from '../../types/databaseUpgrades'
import { supportVariant } from './upgradeHelpers'

/** CurrentVersionCard states what the database runs, how long it is supported and what blocks an upgrade. */
export function CurrentVersionCard({ data }: { data: DatabaseUpgrades }) {
  const { t } = useTranslation('databaseUpgrades')
  const { advice } = data
  const supportLabel =
    advice.support === 'eol_soon'
      ? advice.eol
        ? t('support.eol_soon', { date: advice.eol })
        : t('support.eol_soonNoDate')
      : t(`support.${advice.support}`)
  const advisories = advice.advisories ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('current.title')}</CardTitle>
        <CardDescription>
          {t('current.catalogUpdated', { date: advice.catalog_updated })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium capitalize">{advice.engine}</span>
          <span className="font-mono">{advice.current}</span>
          <Badge variant={supportVariant(advice.support)}>{supportLabel}</Badge>
          {advice.line ? (
            <span className="text-muted-foreground">
              {t('current.line', { line: advice.line })}
            </span>
          ) : null}
        </div>

        {advice.floating ? (
          <p className="text-sm text-muted-foreground">
            {t('current.floating')}
          </p>
        ) : null}
        {!advice.comparable ? (
          <p className="text-sm text-muted-foreground">
            {t('current.notComparable')}
          </p>
        ) : null}

        {advisories.length > 0 ? (
          <div className="space-y-1.5">
            <p className="flex items-center gap-1.5 text-sm font-medium">
              <ShieldWarningIcon className="size-4" aria-hidden="true" />
              {t('current.advisories')}
            </p>
            <div className="flex flex-wrap gap-1.5">
              {advisories.map((id) => (
                <Badge key={id} variant="destructive" className="font-mono">
                  {id}
                </Badge>
              ))}
            </div>
          </div>
        ) : null}

        {advice.manual_reason ? (
          <Alert>
            <InfoIcon aria-hidden="true" />
            <AlertTitle>{t('current.manualReason')}</AlertTitle>
            <AlertDescription>{advice.manual_reason}</AlertDescription>
          </Alert>
        ) : null}
        {advice.notes || advice.note ? (
          <p className="text-sm text-muted-foreground">
            {[advice.notes, advice.note].filter(Boolean).join(' ')}
          </p>
        ) : null}

        {data.blockers.length > 0 ? (
          <Alert variant="destructive">
            <AlertTitle>{t('current.blockersTitle')}</AlertTitle>
            <AlertDescription>
              <ul className="list-disc space-y-0.5 pl-4">
                {data.blockers.map((b) => (
                  <li key={b}>{b}</li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
