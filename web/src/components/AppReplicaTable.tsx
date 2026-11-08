import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from './kit/RelativeTime'
import {
  useAppLoadBalancer,
  useAppLoadBalancerStatus,
  type UpstreamState,
} from '../queries/appLoadBalancer'
import {
  useSetUpstreamAdminState,
  type LiveUpstream,
} from '../queries/loadBalancerLive'

const STATE_VARIANT: Record<
  UpstreamState,
  'success' | 'destructive' | 'muted'
> = {
  healthy: 'success',
  unhealthy: 'destructive',
  draining: 'muted',
  disabled: 'muted',
  unknown: 'muted',
}

// Reads the load balancer status the Scale panel already polls; renders
// nothing until a load balancer is configured and has upstreams.
export function AppReplicaTable({ appName }: { appName: string }) {
  const { t: tc } = useTranslation('common')
  const { t } = useTranslation('resources', { keyPrefix: 'replicaTable' })
  const lb = useAppLoadBalancer(appName)
  const configured = lb.data?.configured === true
  const status = useAppLoadBalancerStatus(appName, configured)
  const setState = useSetUpstreamAdminState(appName)
  const upstreams: LiveUpstream[] | undefined = configured
    ? status.data?.upstreams
    : undefined
  if (!upstreams || upstreams.length === 0) return null

  function toggle(u: LiveUpstream, drain: boolean) {
    setState.mutate(
      { id: u.id, admin_state: drain ? 'draining' : 'active' },
      {
        onError: (err) => {
          toast.add({
            title: t('drainFailed'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table aria-label={tc('a11y.tables.appReplicas')}>
          <TableHeader>
            <TableRow>
              <TableHead>{t('replica')}</TableHead>
              <TableHead>{t('address')}</TableHead>
              <TableHead>{t('health')}</TableHead>
              <TableHead className="text-right">{t('connections')}</TableHead>
              <TableHead className="text-right">{t('weight')}</TableHead>
              <TableHead>{t('lastCheck')}</TableHead>
              <TableHead>{t('drain')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {upstreams.map((u) => {
              const draining =
                u.admin_state === 'draining' || u.state === 'draining'
              return (
                <TableRow key={u.id}>
                  <TableCell>{t('replicaRow', { n: u.replica })}</TableCell>
                  <TableCell className="font-mono text-xs">{u.dial}</TableCell>
                  <TableCell>
                    <Badge variant={STATE_VARIANT[u.state]}>
                      {t(`state.${u.state}`)}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {u.active_connections}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {u.weight}
                  </TableCell>
                  <TableCell>
                    {u.last_check ? (
                      <RelativeTime at={u.last_check} />
                    ) : (
                      t('never')
                    )}
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={draining}
                      disabled={setState.isPending || u.state === 'disabled'}
                      aria-label={t('drainReplica', { n: u.replica })}
                      onCheckedChange={(next) => {
                        toggle(u, next)
                      }}
                    />
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}
