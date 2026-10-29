import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  GlobeGlassIcon,
  LockGlassIcon,
  Share2GlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import type { DatabaseNetwork } from '../../types/databaseAccess'
import { useMakePrivate } from '../../queries/databaseAccess'

function verdictText(
  t: TFunction<'databaseAccess'>,
  n: DatabaseNetwork,
): string {
  const port = n.verdict.port ?? n.published?.host_port
  switch (n.verdict.level) {
    case 'stopped':
      return t('reach.verdict.stopped')
    case 'exposed':
      return t('reach.verdict.exposed', { port })
    case 'restricted':
      return t('reach.verdict.restricted', { port })
    case 'unknown':
      return t('reach.verdict.unknown', { port })
    default:
      if (n.published) return t('reach.verdict.hostOnly', { port })
      if (n.verdict.clients === 0) return t('reach.verdict.privateNone')
      return n.verdict.clients === 1
        ? t('reach.verdict.privateOne')
        : t('reach.verdict.privateMany', { count: n.verdict.clients })
  }
}

type NetworkClient = DatabaseNetwork['clients'][number]

function ClientsTable({ clients }: { clients: DatabaseNetwork['clients'] }) {
  const { t } = useTranslation('databaseAccess')
  const columns = useMemo<ResponsiveColumn<NetworkClient>[]>(
    () => [
      {
        id: 'app',
        header: t('reach.columns.app'),
        primary: true,
        cell: (c) => <span className="font-mono text-xs">{c.app}</span>,
      },
      {
        id: 'via',
        header: t('reach.columns.via'),
        priority: 2,
        cell: (c) => <span className="text-xs">{c.via}</span>,
      },
      {
        id: 'project',
        header: t('reach.columns.project'),
        priority: 3,
        cell: (c) => (
          <span className="text-xs">
            {c.project_name || c.project_id || '-'}
          </span>
        ),
      },
      {
        id: 'environment',
        header: t('reach.columns.environment'),
        priority: 3,
        cell: (c) => <span className="text-xs">{c.environment || '-'}</span>,
      },
      {
        id: 'scope',
        header: t('reach.columns.scope'),
        cell: (c) => (
          <Badge variant={c.in_scope ? 'success' : 'warning'}>
            {c.in_scope ? t('reach.inScope') : t('reach.outOfScope')}
          </Badge>
        ),
      },
    ],
    [t],
  )
  return (
    <ResponsiveTable
      rows={clients}
      columns={columns}
      getRowId={(c) => c.app}
      ariaLabel={t('reach.tableLabel')}
      scrollHeightClass="max-h-[420px]"
    />
  )
}

/** ReachabilityCard answers "who can reach this database" in one verdict, then shows the evidence. */
export function ReachabilityCard({
  databaseName,
  network,
}: {
  databaseName: string
  network: DatabaseNetwork
}) {
  const { t } = useTranslation('databaseAccess')
  const makePrivate = useMakePrivate(databaseName)
  const [confirming, setConfirming] = useState(false)
  const exposed = network.verdict.level === 'exposed'

  function confirm() {
    makePrivate.mutate(undefined, {
      onSuccess: () => {
        toast.add({ title: t('reach.madePrivateToast'), type: 'success' })
        setConfirming(false)
      },
      onError: (err) =>
        toast.add({
          title: t('reach.failedToast'),
          description: err.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card>
      <CardHeader className="space-y-1">
        <CardTitle className="flex items-center gap-1.5 text-sm">
          <Share2GlassIcon className="size-4.5" aria-hidden="true" />
          {t('reach.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <Alert variant={exposed ? 'destructive' : 'default'}>
          {exposed ? (
            <GlobeGlassIcon size="1.2em" aria-hidden="true" />
          ) : (
            <LockGlassIcon size="1.2em" aria-hidden="true" />
          )}
          <AlertTitle>{verdictText(t, network)}</AlertTitle>
          {network.published ? (
            <AlertDescription className="flex flex-wrap items-center gap-2">
              <span>
                {t('reach.bind', { bind: network.published.bind.join(', ') })}
              </span>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setConfirming(true)}
              >
                {t('reach.makePrivate')}
              </Button>
            </AlertDescription>
          ) : null}
        </Alert>
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <div>
            <dt className="text-xs font-medium text-muted-foreground">
              {t('reach.internal')}
            </dt>
            <dd className="font-mono text-xs">
              {network.internal.host}:{network.internal.port}
              {network.internal.address ? ` (${network.internal.address})` : ''}
            </dd>
          </div>
          <div>
            <dt className="text-xs font-medium text-muted-foreground">
              {t('reach.published')}
            </dt>
            <dd className="text-xs">
              {network.published
                ? `${network.published.host_port} -> ${network.published.container_port}`
                : t('reach.notPublished')}
            </dd>
          </div>
          <div className="sm:col-span-2">
            <dt className="text-xs font-medium text-muted-foreground">
              {t('reach.networks')}
            </dt>
            <dd className="mt-1 flex flex-wrap gap-1.5">
              {network.networks.map((n) => (
                <Badge key={n.name} variant="muted">
                  {n.name} ({t(`reach.networkKind.${n.kind}`)})
                </Badge>
              ))}
            </dd>
          </div>
        </dl>
        <div className="space-y-2">
          <h3 className="text-xs font-medium">{t('reach.clients')}</h3>
          {network.clients.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('reach.noClients')}
            </p>
          ) : (
            <ClientsTable clients={network.clients} />
          )}
        </div>
        {network.caveats.includes('bridge_reachable') ? (
          <p className="text-xs text-muted-foreground">
            {t('reach.bridgeCaveat')}
          </p>
        ) : null}
      </CardContent>
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('reach.makePrivateTitle')}</DialogTitle>
            <DialogDescription>{t('reach.makePrivateBody')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirming(false)}>
              {t('users.cancel')}
            </Button>
            <Button disabled={makePrivate.isPending} onClick={confirm}>
              {t('reach.makePrivateConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
