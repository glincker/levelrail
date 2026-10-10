import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  GlobeHemisphereWestIcon,
  LockKeyIcon,
  ShareNetworkIcon,
} from '@phosphor-icons/react/dist/ssr'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import type { DatabaseNetwork } from '../../types/databaseAccess'
import { useMakePrivate } from '../../queries/databaseAccess'
import { useWindowedRows } from '../../lib/useWindowedRows'

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

function ClientsTable({ clients }: { clients: DatabaseNetwork['clients'] }) {
  const { t } = useTranslation('databaseAccess')
  const { parentRef, windowed, indices, padTop, padBottom } = useWindowedRows(
    clients.length,
  )
  return (
    <div
      ref={parentRef}
      className={windowed ? 'max-h-[420px] overflow-auto' : 'overflow-auto'}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('reach.columns.app')}</TableHead>
            <TableHead>{t('reach.columns.via')}</TableHead>
            <TableHead>{t('reach.columns.project')}</TableHead>
            <TableHead>{t('reach.columns.environment')}</TableHead>
            <TableHead>{t('reach.columns.scope')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {padTop > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell colSpan={5} style={{ height: padTop }} />
            </TableRow>
          ) : null}
          {indices.map((i) => {
            const c = clients[i]
            if (!c) return null
            return (
              <TableRow key={c.app}>
                <TableCell className="font-mono text-xs">{c.app}</TableCell>
                <TableCell className="text-xs">{c.via}</TableCell>
                <TableCell className="text-xs">
                  {c.project_name || c.project_id || '-'}
                </TableCell>
                <TableCell className="text-xs">
                  {c.environment || '-'}
                </TableCell>
                <TableCell>
                  <Badge variant={c.in_scope ? 'success' : 'warning'}>
                    {c.in_scope ? t('reach.inScope') : t('reach.outOfScope')}
                  </Badge>
                </TableCell>
              </TableRow>
            )
          })}
          {padBottom > 0 ? (
            <TableRow aria-hidden="true">
              <TableCell colSpan={5} style={{ height: padBottom }} />
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
    </div>
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
          <ShareNetworkIcon className="size-4" aria-hidden="true" />
          {t('reach.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <Alert variant={exposed ? 'destructive' : 'default'}>
          {exposed ? (
            <GlobeHemisphereWestIcon aria-hidden="true" />
          ) : (
            <LockKeyIcon aria-hidden="true" />
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
