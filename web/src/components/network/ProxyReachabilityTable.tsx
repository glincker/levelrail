import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  GlobeGlassIcon,
} from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
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
  const appList = useAppListOptional()
  const unreachableCount = domains.filter((d) => !d.reachable).length
  const columns = useMemo<ResponsiveColumn<NetworkProxyDomain>[]>(
    () => [
      {
        id: 'domain',
        header: t('table.domain'),
        primary: true,
        cell: (row) => (
          <Link
            to="/apps/$name/domains"
            params={{ name: row.app }}
            className="font-mono text-sm break-all text-foreground hover:underline"
          >
            {row.domain}
          </Link>
        ),
      },
      {
        id: 'app',
        header: t('table.app'),
        priority: 2,
        cell: (row) => (
          <Link
            to="/apps/$name"
            params={{ name: row.app }}
            className="text-sm text-foreground hover:underline"
          >
            {row.app}
          </Link>
        ),
      },
      {
        id: 'node',
        header: t('table.node'),
        priority: 3,
        cell: (row) => (
          <span className="text-sm text-muted-foreground">
            {row.is_local_node
              ? 'This control plane (local)'
              : row.node_name || row.node_id || '-'}
          </span>
        ),
      },
      {
        id: 'reachable',
        header: t('table.reachable'),
        cell: (row) => <ReachableBadge row={row} />,
      },
      {
        id: 'port',
        header: t('table.port'),
        priority: 3,
        cell: (row) => (
          <span className="font-mono text-sm text-muted-foreground">
            :{row.port}
          </span>
        ),
      },
      {
        id: 'tls',
        header: t('table.tls'),
        priority: 2,
        cell: (row) => <TlsCell row={row} />,
      },
    ],
    [t],
  )

  if (domains.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<GlobeGlassIcon className="size-6" />}
        title={t('empty.title')}
        description={t('empty.description')}
      />
    )
  }

  return (
    <div className="space-y-3">
      {unreachableCount > 0 ? (
        <Alert variant="destructive">
          <CircleAlertGlassIcon className="text-tone-danger" size="1.4em" />
          <AlertTitle>{t('reachable.no')}</AlertTitle>
          <AlertDescription>
            {t('unreachableBanner', { count: unreachableCount })}
          </AlertDescription>
        </Alert>
      ) : null}

      <ResponsiveTable
        rows={domains}
        columns={columns}
        getRowId={(row) => `${row.app}:${row.domain}`}
        ariaLabel={t('table.label')}
        renderActions={(row) => {
          if (row.reachable) return null
          const app = appList.data?.find((a) => a.name === row.app)
          return (
            <div className="flex max-w-3xs flex-col items-end gap-1">
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
          )
        }}
      />
    </div>
  )
}

function ReachableBadge({ row }: { row: NetworkProxyDomain }) {
  const { t } = useTranslation('networkProxy')
  if (row.reachable) {
    return (
      <Badge variant="success" className="gap-1">
        <CircleCheckGlassIcon
          className="size-4.5 text-tone-success"
          aria-hidden="true"
        />
        {t('reachable.yes')}
      </Badge>
    )
  }
  return (
    <Badge variant="destructive" className="gap-1" title={row.fix_command}>
      <CircleAlertGlassIcon
        className="size-4.5 text-tone-danger"
        aria-hidden="true"
      />
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
