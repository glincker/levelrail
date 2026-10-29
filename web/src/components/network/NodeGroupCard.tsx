import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CopyGlassIcon,
  DatabaseGlassIcon,
  LayersGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type {
  NetworkTopologyApp,
  NetworkTopologyConnection,
  NetworkTopologyDatabase,
  NetworkTopologyGroup,
} from '../../types/networkTopology'

const STATUS_VARIANT: Record<string, 'success' | 'destructive' | 'muted'> = {
  online: 'success',
  offline: 'destructive',
  pending: 'muted',
}

function CopyAddress({ value }: { value: string }) {
  const { t } = useTranslation('network')
  const [copied, setCopied] = useState(false)
  function copy() {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }
  return (
    <Button
      type="button"
      size="icon-xs"
      variant="ghost"
      onClick={copy}
      aria-label={t('resource.copy')}
      title={copied ? t('resource.copied') : t('resource.copy')}
    >
      {copied ? (
        <CheckGlassIcon className="size-5" aria-hidden="true" />
      ) : (
        <CopyGlassIcon className="size-5" aria-hidden="true" />
      )}
    </Button>
  )
}

interface ResourceRowProps {
  kind: 'app' | 'database'
  name: string
  dnsName?: string
  port?: number
  meshAddress?: string
  meshEnabled: boolean
  connections: NetworkTopologyConnection[]
  detail?: string
}

function ResourceRow({
  kind,
  name,
  dnsName,
  port,
  meshAddress,
  meshEnabled,
  connections,
  detail,
}: ResourceRowProps) {
  const { t } = useTranslation('network')
  const Icon = kind === 'app' ? LayersGlassIcon : DatabaseGlassIcon
  const address = dnsName ? (port ? `${dnsName}:${port}` : dnsName) : ''
  const label = kind === 'app' ? t('resource.app') : t('resource.database')
  const missingMesh = meshEnabled && !meshAddress

  const title =
    kind === 'app' ? (
      <Link
        to="/apps/$name/overview"
        params={{ name }}
        className="truncate font-medium text-foreground hover:underline"
      >
        {name}
      </Link>
    ) : (
      <Link
        to="/databases/$name"
        params={{ name }}
        className="truncate font-medium text-foreground hover:underline"
      >
        {name}
      </Link>
    )

  return (
    <li className="flex flex-col gap-1 rounded-lg border border-border bg-card px-3 py-2 text-sm">
      <div className="flex min-w-0 items-center gap-1.5">
        <Icon
          className="size-3.5 shrink-0 text-muted-foreground"
          aria-label={label}
        />
        {title}
        {detail ? (
          <span className="shrink-0 text-xs text-muted-foreground">
            {detail}
          </span>
        ) : null}
        {missingMesh ? (
          <Badge variant="warning" className="ml-auto shrink-0">
            <CircleAlertGlassIcon
              className="size-4.5 text-tone-danger"
              aria-hidden="true"
            />
            {t('resource.noMeshAddress')}
          </Badge>
        ) : (
          <Badge variant="outline" className="ml-auto shrink-0">
            {meshAddress ?? t('resource.local')}
          </Badge>
        )}
      </div>
      {address ? (
        <div className="flex min-w-0 items-center gap-1">
          <span className="truncate font-mono text-xs text-muted-foreground">
            {address}
          </span>
          <CopyAddress value={address} />
        </div>
      ) : null}
      {connections.map((c) => (
        <span
          key={`${c.app}:${c.database}:${c.env_var ?? ''}`}
          className="truncate text-xs text-muted-foreground"
        >
          {t('resource.connectsTo', { database: c.database })}
          {c.env_var ? ` (${c.env_var})` : ''}
        </span>
      ))}
    </li>
  )
}

export function NodeGroupCard({
  group,
  meshEnabled,
  connections,
}: {
  group: NetworkTopologyGroup
  meshEnabled: boolean
  connections: NetworkTopologyConnection[]
}) {
  const { t } = useTranslation('network')
  const isLocal = group.kind === 'local'
  const isUnplaced = group.kind === 'unplaced'
  const title = isLocal
    ? t('group.thisServer')
    : isUnplaced
      ? t('group.unplaced')
      : group.region?.trim() || group.name
  const empty = group.apps.length === 0 && group.databases.length === 0
  const secondary = isLocal
    ? [group.hostname, group.address].filter(Boolean).join(' · ')
    : group.mesh_address || ''

  const heading = (
    <span className="flex min-w-0 items-center gap-1.5 text-sm font-semibold text-foreground">
      <ServerGlassIcon
        className="size-5 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <span className="truncate">{title}</span>
    </span>
  )

  return (
    <section
      className="flex min-w-0 flex-col gap-3 rounded-xl border border-border bg-muted/20 p-3"
      aria-label={title}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        {group.node_id && !isLocal ? (
          <Link
            to="/nodes/$id"
            params={{ id: group.node_id }}
            className="min-w-0 hover:underline"
          >
            {heading}
          </Link>
        ) : (
          heading
        )}
        <div className="flex items-center gap-1.5">
          {group.status ? (
            <Badge variant={STATUS_VARIANT[group.status] ?? 'muted'}>
              {group.status}
            </Badge>
          ) : null}
          {!group.schedulable ? (
            <Badge variant="warning">{t('group.cordoned')}</Badge>
          ) : null}
        </div>
      </div>

      {isUnplaced ? (
        <div className="space-y-1 text-xs text-muted-foreground">
          <p>
            {t('group.unplacedHint', {
              ids: (group.stale_node_ids ?? []).join(', '),
            })}
          </p>
          <p>{t('group.moveHint')}</p>
          <Link
            to="/nodes"
            className="font-medium text-foreground underline underline-offset-4"
          >
            {t('group.reAdd')}
          </Link>
        </div>
      ) : secondary ? (
        <p className="truncate font-mono text-xs text-muted-foreground">
          {secondary}
        </p>
      ) : meshEnabled && !isLocal ? (
        <p className="text-xs text-muted-foreground">
          {t('group.noMeshAddress')}
        </p>
      ) : null}

      {empty ? (
        <p className="text-xs text-muted-foreground">{t('group.empty')}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {group.apps.map((app: NetworkTopologyApp) => (
            <ResourceRow
              key={`app:${app.name}`}
              kind="app"
              name={app.name}
              dnsName={app.dns_name}
              port={app.port}
              meshAddress={app.mesh_address}
              meshEnabled={meshEnabled && !isUnplaced}
              connections={connections.filter((c) => c.app === app.name)}
              detail={app.domains?.[0]}
            />
          ))}
          {group.databases.map((db: NetworkTopologyDatabase) => (
            <ResourceRow
              key={`db:${db.name}`}
              kind="database"
              name={db.name}
              dnsName={db.dns_name}
              meshAddress={db.mesh_address}
              meshEnabled={meshEnabled && !isUnplaced}
              connections={[]}
              detail={db.engine}
            />
          ))}
        </ul>
      )}
    </section>
  )
}
