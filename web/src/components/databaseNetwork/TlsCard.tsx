import { useTranslation } from 'react-i18next'
import { LockSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import type { DatabaseNetwork } from '../../types/databaseAccess'
import {
  useDatabaseNetwork,
  useSetRequireTls,
  useSetTlsEnabled,
} from '../../queries/databaseAccess'

/** TlsCard toggles whether the server refuses connections that are not encrypted. */
export function TlsCard({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  const { t } = useTranslation('databaseAccess')
  const setTls = useSetRequireTls(databaseName)
  const switchTls = useSetTlsEnabled(databaseName)
  const recheck = useDatabaseNetwork(databaseName)
  const { tls } = network
  const always = tls.supported === false && tls.required
  const disabledReason = always
    ? t('tls.redis')
    : !tls.supported
      ? t('tls.unsupported')
      : !tls.enabled
        ? t('tls.notEnabled')
        : null

  function change(require: boolean) {
    setTls.mutate(require, {
      onSuccess: () =>
        toast.add({ title: t('tls.updatedToast'), type: 'success' }),
      onError: (err) =>
        toast.add({
          title: t('tls.failedToast'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  function switchEnabled(enabled: boolean) {
    switchTls.mutate(enabled, {
      onSuccess: () =>
        toast.add({ title: t('tls.switchedToast'), type: 'success' }),
      onError: (err) =>
        toast.add({
          title: t('tls.switchFailedToast'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <LockSimpleIcon className="size-4" aria-hidden="true" />
          {t('tls.title')}
          <Badge variant={tls.required ? 'success' : 'muted'}>
            {tls.required ? t('tls.required') : t('tls.optional')}
          </Badge>
        </CardTitle>
        <p className="text-xs text-muted-foreground">{t('tls.description')}</p>
      </CardHeader>
      <CardContent className="space-y-2">
        {tls.toggleable ? (
          <div className="space-y-1">
            <label className="flex items-center gap-3 text-sm">
              <Switch
                checked={tls.enabled}
                disabled={switchTls.isPending}
                onCheckedChange={switchEnabled}
                aria-label={t('tls.serverTitle')}
              />
              <span>{t('tls.serverTitle')}</span>
            </label>
            <p className="text-xs text-muted-foreground">
              {t('tls.serverHint')}
            </p>
          </div>
        ) : null}
        <label className="flex items-center gap-3 text-sm">
          <Switch
            checked={tls.required}
            disabled={disabledReason !== null || setTls.isPending}
            onCheckedChange={change}
            aria-label={t('tls.title')}
          />
          <span>{t('tls.title')}</span>
        </label>
        {disabledReason ? (
          <p className="text-xs text-muted-foreground">{disabledReason}</p>
        ) : (
          <p className="text-xs text-muted-foreground">{t('tls.warning')}</p>
        )}
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <span className="text-xs text-muted-foreground">
            {t('tls.status', {
              state: tls.state || (tls.enabled ? t('tls.on') : t('tls.off')),
            })}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={recheck.isFetching}
            onClick={() => void recheck.refetch()}
          >
            {t('tls.recheck')}
          </Button>
        </div>
        {tls.drift ? (
          <p className="text-xs text-destructive" role="alert">
            {t('tls.drift')}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
