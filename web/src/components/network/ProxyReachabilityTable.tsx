import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  GlobeIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { EmptyState } from '@/components/ui/empty-state'
import { MoveToNodeDialog } from '../MoveToNodeDialog'
import { certStatusMeta } from '../../lib/certStatus'
import { useAppListOptional } from '../../queries/apps'
import type { NetworkProxyDomain } from '../../queries/networkProxy'

// The table body for the Traffic page: one row per routed domain, each
// showing the exact fact tonight's live-VPS bug made invisible (an app's
// domain served by this control plane's own ingress, while the app
// itself actually runs somewhere that ingress can never reach) plus a
// direct fix, rather than only a doctor pass/fail line.
export function ProxyReachabilityTable({
  domains,
}: {
  domains: NetworkProxyDomain[]
}) {
  const { t } = useTranslation('networkProxy')
  const unreachableCount = domains.filter((d) => !d.reachable).length

  if (domains.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<GlobeIcon className="size-5" />}
        title={t('empty.title')}
        description={t('empty.description')}
      />
    )
  }

  return (
    <div className="space-y-3">
      {unreachableCount > 0 ? (
        <Alert variant="destructive">
          <WarningCircleIcon />
          <AlertTitle>{t('reachable.no')}</AlertTitle>
          <AlertDescription>
            {t('unreachableBanner', { count: unreachableCount })}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('table.domain')}</TableHead>
              <TableHead>{t('table.app')}</TableHead>
              <TableHead>{t('table.node')}</TableHead>
              <TableHead>{t('table.reachable')}</TableHead>
              <TableHead>{t('table.port')}</TableHead>
              <TableHead>{t('table.tls')}</TableHead>
              <TableHead aria-hidden="true" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {domains.map((row) => (
              <ProxyDomainRow key={`${row.app}:${row.domain}`} row={row} />
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}

function ProxyDomainRow({ row }: { row: NetworkProxyDomain }) {
  const { t } = useTranslation('networkProxy')
  const appList = useAppListOptional()
  const app = appList.data?.find((a) => a.name === row.app)

  return (
    <TableRow>
      <TableCell className="min-w-0">
        <Link
          to="/apps/$name/domains"
          params={{ name: row.app }}
          className="truncate font-mono text-sm text-foreground hover:underline"
        >
          {row.domain}
        </Link>
      </TableCell>
      <TableCell>
        <Link
          to="/apps/$name"
          params={{ name: row.app }}
          className="text-sm text-foreground hover:underline"
        >
          {row.app}
        </Link>
      </TableCell>
      <TableCell className="text-sm text-muted-foreground">
        {row.is_local_node
          ? 'This control plane (local)'
          : row.node_name || row.node_id || '-'}
      </TableCell>
      <TableCell>
        <ReachableBadge row={row} />
      </TableCell>
      <TableCell className="font-mono text-sm text-muted-foreground">
        :{row.port}
      </TableCell>
      <TableCell>
        <TlsCell row={row} />
      </TableCell>
      <TableCell className="max-w-3xs text-right whitespace-normal">
        {!row.reachable ? (
          <div className="flex flex-col items-end gap-1">
            <MoveToNodeDialog
              kind="app"
              name={row.app}
              currentNodeId={row.node_id}
              volumeCount={app?.volumes?.length ?? 0}
              volumeNames={app?.volumes?.map((v) => v.name) ?? []}
            />
            <span className="text-right text-[11px] text-muted-foreground">
              {row.reason ?? t('fix.explanation')}
            </span>
          </div>
        ) : null}
      </TableCell>
    </TableRow>
  )
}

function ReachableBadge({ row }: { row: NetworkProxyDomain }) {
  const { t } = useTranslation('networkProxy')
  if (row.reachable) {
    return (
      <Badge variant="success" className="gap-1">
        <CheckCircleIcon className="size-3" aria-hidden="true" />
        {t('reachable.yes')}
      </Badge>
    )
  }
  return (
    <Badge variant="destructive" className="gap-1" title={row.fix_command}>
      <WarningCircleIcon className="size-3" aria-hidden="true" />
      {t('reachable.no')}
    </Badge>
  )
}

function TlsCell({ row }: { row: NetworkProxyDomain }) {
  const { t } = useTranslation('networkProxy')
  if (!row.tls_status) {
    return <span className="text-xs text-muted-foreground/60 italic">-</span>
  }
  const meta = certStatusMeta[row.tls_status]
  return (
    <div className="flex flex-col items-start gap-0.5">
      <Badge variant={meta.variant}>{meta.label}</Badge>
      {row.tls_source ? (
        <span className="text-[11px] text-muted-foreground">
          {row.tls_source === 'custom'
            ? t('tlsSource.custom')
            : t('tlsSource.acme')}
          {row.tls_issuer ? ` · ${row.tls_issuer}` : ''}
        </span>
      ) : null}
    </div>
  )
}
