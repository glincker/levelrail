import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '@/lib/apiError'
import { useDatabaseUpgrades } from '../../queries/databaseUpgrades'
import { CurrentVersionCard } from './CurrentVersionCard'
import { HistoryCard } from './HistoryCard'
import { PolicyCard } from './PolicyCard'
import { TargetsCard } from './TargetsCard'

const NOT_CONFIGURED_STATUS = 501

/** UpgradesPanel is the database detail Upgrades tab: version and support, targets, policy, then history. */
export function UpgradesPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databaseUpgrades')
  const { data, isLoading, error } = useDatabaseUpgrades(databaseName)
  const unavailable =
    error instanceof ApiError && error.status === NOT_CONFIGURED_STATUS

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold">{t('title')}</h2>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </div>
      {isLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : unavailable ? (
        <Alert>
          <AlertTitle>{t('unavailableTitle')}</AlertTitle>
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : error || !data ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('loadFailed', { error: error?.message ?? '' })}
          </AlertDescription>
        </Alert>
      ) : (
        <>
          <CurrentVersionCard data={data} />
          <TargetsCard databaseName={databaseName} data={data} />
          <PolicyCard databaseName={databaseName} data={data} />
          <HistoryCard active={data.active} history={data.history} />
        </>
      )}
    </div>
  )
}
