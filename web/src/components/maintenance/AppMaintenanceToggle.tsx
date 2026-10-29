import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useAppMaintenance,
  useClearAppMaintenance,
  useSetAppMaintenance,
} from '../../queries/appMaintenance'
import { useFleetMaintenance } from '../../queries/domainMaintenance'
import { EnableMaintenanceDialog } from './EnableMaintenanceDialog'

// Turns maintenance on or off for every domain of the app. The server applies
// it to all of them or none, so a failure leaves nothing half done.
export function AppMaintenanceToggle({ appName }: { appName: string }) {
  const { t } = useTranslation('maintenance')
  const { data } = useAppMaintenance(appName)
  const { data: fleet } = useFleetMaintenance()
  const set = useSetAppMaintenance(appName)
  const clear = useClearAppMaintenance(appName)
  const [confirming, setConfirming] = useState(false)
  if (!data) return null
  const none = data.total_domains === 0
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="min-w-0">
        <p className="font-medium">{t('appToggle.title')}</p>
        <p className="text-xs text-muted-foreground">
          {none ? t('appToggle.noDomains') : t('appToggle.description')}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant={data.enabled ? 'warning' : 'muted'}>
          <WrenchGlassIcon className="size-3.5" />
          {data.enabled
            ? t('appToggle.on', {
                active: data.active_domains,
                total: data.total_domains,
              })
            : t('appToggle.off')}
        </Badge>
        {data.enabled ? (
          <Button
            size="sm"
            variant="outline"
            disabled={clear.isPending}
            onClick={() => {
              clear.mutate()
            }}
          >
            {clear.isPending
              ? t('appToggle.disabling')
              : t('appToggle.disable')}
          </Button>
        ) : (
          <Button
            size="sm"
            variant="outline"
            disabled={none || set.isPending}
            onClick={() => {
              setConfirming(true)
            }}
          >
            {set.isPending ? t('appToggle.enabling') : t('appToggle.enable')}
          </Button>
        )}
      </div>
      <EnableMaintenanceDialog
        domain={appName}
        request={confirming ? {} : null}
        defaultMaxSeconds={fleet?.max_duration_seconds ?? 0}
        pending={set.isPending}
        error={
          set.isError
            ? t('appToggle.failed', { message: set.error.message })
            : null
        }
        onCancel={() => {
          setConfirming(false)
        }}
        onConfirm={(req) => {
          set.mutate(req, {
            onSuccess: () => {
              setConfirming(false)
              toast.add({ title: t('appToggle.saved'), type: 'success' })
            },
          })
        }}
      />
    </div>
  )
}
