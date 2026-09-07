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

// Maintenance mode for one already-saved domain: while enabled, the
// embedded Caddy ingress serves a fixed "down for maintenance" response
// instead of proxying to the container, without stopping it
// (internal/reconcile/ingress). Unlike DomainBasicAuthControl, there is
// no credential to collect, so this is a direct toggle, not an
// expand-to-form control: the action is instantly reversible (flip it
// back off any time, no data lost), so it doesn't need
// RotateMasterKeyDialog-level confirmation friction either.
export function DomainMaintenanceControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data: maintenance, isLoading } = useDomainMaintenance(appName, domain)
  const setMaintenance = useSetDomainMaintenance(appName, domain)
  const clearMaintenance = useClearDomainMaintenance(appName, domain)

  if (isLoading) {
    return (
      <div
        className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3"
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
          <Badge variant="warning" className="shrink-0">
            <WrenchIcon className="size-3" />
            Maintenance mode on
          </Badge>
        ) : (
          <Badge variant="muted" className="shrink-0">
            <WrenchIcon className="size-3" />
            Not in maintenance
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
            {clearMaintenance.isPending ? 'Ending...' : 'End maintenance'}
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
            {setMaintenance.isPending ? 'Enabling...' : 'Enable maintenance mode'}
          </Button>
        )}
      </div>
      {enabled ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Visitors to {domain} see a fixed maintenance page. The container keeps
          running; nothing here stops or restarts it.
        </p>
      ) : null}
      {error ? (
        <Alert variant="destructive" className="mt-2">
          <WarningIcon />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
