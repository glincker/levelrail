import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { CACHE_TLS_PORT } from '@/lib/cacheAccess'
import {
  useDatabaseNetwork,
  useSetTlsOptional,
} from '../../../queries/databaseAccess'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { RevealCredentialCard } from '../../databaseAccess/RevealCredentialCard'
import { CopyField } from '../../traffic/CopyField'

// Host, ports and password for Redis, Valkey and KeyDB. The password reuses the
// audited reveal card, so it never travels in the resource payload.
export function CacheConnectionCard({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const network = useDatabaseNetwork(database.name)
  const switchTls = useSetTlsOptional(database.name)
  const internal = network.data?.internal
  const tlsOn = network.data?.tls.enabled ?? database.tls_enabled ?? false
  const tlsRequired = network.data?.tls.required ?? false
  const canOptional = tlsOn && tlsRequired

  function makeOptional() {
    switchTls.mutate(undefined, {
      onSuccess: () =>
        toast.add({ title: t('cacheConnect.optionalToast'), type: 'success' }),
      onError: (err) =>
        toast.add({
          title: t('cacheConnect.optionalFailed'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle>{t('cacheConnect.title')}</CardTitle>
            <Badge variant={tlsOn ? 'success' : 'muted'}>
              {tlsOn
                ? tlsRequired
                  ? t('cacheConnect.tlsRequired')
                  : t('cacheConnect.tlsOptional')
                : t('connection.tlsOff')}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {internal ? (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <CopyField value={internal.host} label={t('cacheConnect.host')} />
              <CopyField
                value={String(internal.port)}
                label={t('cacheConnect.port')}
              />
              {tlsOn ? (
                <CopyField
                  value={String(CACHE_TLS_PORT)}
                  label={t('cacheConnect.tlsPort')}
                />
              ) : null}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('connection.unavailable')}
            </p>
          )}
          {canOptional ? (
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3">
              <p className="min-w-0 text-sm text-muted-foreground">
                {t('cacheConnect.optionalHint')}
              </p>
              <Button
                size="sm"
                variant="outline"
                disabled={switchTls.isPending}
                onClick={makeOptional}
              >
                {t('cacheConnect.makeOptional')}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>
      <RevealCredentialCard databaseName={database.name} />
    </div>
  )
}
