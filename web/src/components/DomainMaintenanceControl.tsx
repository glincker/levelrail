import { useTranslation } from 'react-i18next'
import { WarningIcon, WrenchIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useClearDomainMaintenance,
  useDomainMaintenance,
  useSetDomainMaintenance,
} from '../queries/domainMaintenance'
import { EnforcedBadge, EnforcementNote } from './domains/EnforcementGate'
import { useDomainEnforcement } from '../hooks/useDomainEnforcement'
import { isEnforced } from '../lib/domainEnforcement'

// Maintenance mode for one saved domain. The "on" state is only shown when
// the ingress that visitors reach really serves the maintenance page.
export function DomainMaintenanceControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const { data: maintenance, isLoading } = useDomainMaintenance(appName, domain)
  const setMaintenance = useSetDomainMaintenance(appName, domain)
  const clearMaintenance = useClearDomainMaintenance(appName, domain)
  const enforcement = useDomainEnforcement(domain)

  if (isLoading) {
    return (
      <div
        className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
        aria-hidden="true"
      >
        <Skeleton className="h-5 w-28 rounded-full" />
        <Skeleton className="h-7 w-32" />
      </div>
    )
  }

  const enabled = maintenance?.enabled ?? false
  const pending = setMaintenance.isPending || clearMaintenance.isPending
  const error = setMaintenance.isError
    ? setMaintenance.error.message
    : clearMaintenance.isError
      ? clearMaintenance.error.message
      : null

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        {enabled ? (
          <EnforcedBadge domain={domain} control="maintenance">
            <Badge variant="warning" className="shrink-0">
              <WrenchIcon className="size-3" />
              {t('maintenance.on')}
            </Badge>
          </EnforcedBadge>
        ) : (
          <Badge variant="muted" className="shrink-0">
            <WrenchIcon className="size-3" />
            {t('maintenance.off')}
          </Badge>
        )}
        {enabled ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={pending}
            onClick={() => {
              clearMaintenance.mutate()
            }}
          >
            {clearMaintenance.isPending
              ? t('maintenance.ending')
              : t('maintenance.end')}
          </Button>
        ) : (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={pending}
            onClick={() => {
              setMaintenance.mutate()
            }}
          >
            {setMaintenance.isPending
              ? t('maintenance.enabling')
              : t('maintenance.enable')}
          </Button>
        )}
      </div>
      {enabled && isEnforced(enforcement, 'maintenance') ? (
        <p className="mt-2 text-xs text-muted-foreground">
          {t('maintenance.activeHint', { domain })}
        </p>
      ) : null}
      <EnforcementNote domain={domain} control="maintenance" active={enabled} />
      {error ? (
        <Alert variant="destructive" className="mt-2">
          <WarningIcon />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
