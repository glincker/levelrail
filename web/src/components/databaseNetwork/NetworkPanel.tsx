import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { useDatabaseNetwork } from '../../queries/databaseAccess'
import { AllowedSourcesCard } from './AllowedSourcesCard'
import { ReachabilityCard } from './ReachabilityCard'
import { ScopeCard } from './ScopeCard'
import { TlsCard } from './TlsCard'

/** NetworkPanel is the database detail Network tab: reachability first, then the controls that change it. */
export function NetworkPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databaseAccess')
  const { data, isLoading, error } = useDatabaseNetwork(databaseName)

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold">{t('network.title')}</h2>
        <p className="text-sm text-muted-foreground">
          {t('network.description')}
        </p>
      </div>
      {isLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : error || !data ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('network.loadFailed', { error: error?.message ?? '' })}
          </AlertDescription>
        </Alert>
      ) : (
        <>
          <ReachabilityCard databaseName={databaseName} network={data} />
          <AllowedSourcesCard databaseName={databaseName} network={data} />
          <ScopeCard
            key={data.scope.current}
            databaseName={databaseName}
            network={data}
          />
          <TlsCard databaseName={databaseName} network={data} />
        </>
      )}
    </div>
  )
}
