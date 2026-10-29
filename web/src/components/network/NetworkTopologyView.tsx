import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Share2GlassIcon } from '@thesvg/ui-react'
import type { NetworkTopologyResponse } from '../../types/networkTopology'
import { EmptyState } from '@/components/ui/empty-state'
import { NodeGroupCard } from './NodeGroupCard'

function SummaryTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="truncate text-sm font-semibold text-foreground">{value}</p>
    </div>
  )
}

export function NetworkSummaryRow({
  topology,
  restartPending,
}: {
  topology: NetworkTopologyResponse
  restartPending: boolean
}) {
  const { t } = useTranslation('network')
  const servers = topology.groups.filter((g) => g.kind !== 'unplaced').length
  const resources = topology.apps.length + topology.databases.length
  const mesh = topology.mesh_enabled
    ? t('summary.meshOn')
    : restartPending
      ? t('summary.meshRestart')
      : t('summary.meshOff')
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      <SummaryTile label={t('summary.nodes')} value={String(servers)} />
      <SummaryTile label={t('summary.mesh')} value={mesh} />
      <SummaryTile label={t('summary.zone')} value={topology.zone} />
      <SummaryTile label={t('summary.resources')} value={String(resources)} />
    </div>
  )
}

function Legend() {
  const { t } = useTranslation('network')
  return (
    <details className="rounded-lg border border-border bg-card/50 px-3 py-2 text-xs text-muted-foreground">
      <summary className="cursor-pointer font-medium text-foreground">
        {t('legend.title')}
      </summary>
      <ul className="mt-2 list-disc space-y-1 pl-4">
        <li>{t('legend.server')}</li>
        <li>{t('legend.internalName')}</li>
        <li>{t('legend.local')}</li>
        <li>{t('legend.mesh')}</li>
      </ul>
    </details>
  )
}

// Node-grouped list. The API already placed every resource: this server,
// a known node, or unplaced (internal/api/network_topology_groups.go).
export function NetworkTopologyView({
  topology,
}: {
  topology: NetworkTopologyResponse
}) {
  const { t } = useTranslation('network')
  const empty =
    topology.apps.length === 0 &&
    topology.databases.length === 0 &&
    topology.groups.length <= 1

  if (empty) {
    return (
      <EmptyState
        icon={<Share2GlassIcon className="size-5.5" />}
        title={t('empty.title')}
        description={t('empty.description')}
        action={
          <Link
            to="/apps"
            className="text-sm font-medium text-foreground underline underline-offset-4"
          >
            {t('empty.action')}
          </Link>
        }
      />
    )
  }

  return (
    <div className="space-y-3">
      <Legend />
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {topology.groups.map((group) => (
          <NodeGroupCard
            key={group.key}
            group={group}
            meshEnabled={topology.mesh_enabled}
            connections={topology.connections}
          />
        ))}
      </div>
    </div>
  )
}
