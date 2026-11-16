import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { StatusPill } from '../kit/StatusPill'
import type { Tone } from '../kit/tone'
import { SkeletonList } from '../kit/Skeleton'
import { RelativeTime } from '../kit/RelativeTime'
import { DestroyServerAction } from '../nodes/DestroyServerAction'
import { useNodeProvisions } from '../../queries/nodeProvision'
import type { NodeProvisionStatus } from '../../types/nodeProvision'
import type { SectionProps } from './sections'

const STATUS_TONE: Record<NodeProvisionStatus, Tone> = {
  creating: 'info',
  booting: 'info',
  installing: 'info',
  enrolling: 'warning',
  ready: 'success',
  failed: 'danger',
  destroying: 'warning',
  destroyed: 'neutral',
}

const TH = 'px-2 py-1.5 text-start text-xs font-medium text-muted-foreground'
const TD = 'px-2 py-2 align-middle text-sm'

export function ProviderServersSection({ provider, labels }: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const provisions = useNodeProvisions()
  const rows = (provisions.data ?? []).filter(
    (p) => p.provider === provider.id && p.status !== 'destroyed',
  )

  return (
    <section aria-labelledby="cp-servers" className="space-y-2">
      <div className="flex flex-wrap items-baseline gap-x-3">
        <h2 id="cp-servers" className="text-sm font-semibold">
          {labels.servers}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t('servers.description', { name: labels.name })}
        </p>
      </div>
      {provisions.isPending ? (
        <SkeletonList rows={2} />
      ) : provisions.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('servers.loadError', { message: provisions.error.message })}
          </AlertDescription>
        </Alert>
      ) : rows.length === 0 ? (
        <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
          {t('servers.empty', { servers: labels.servers })}{' '}
          {t('servers.emptyHint', { create: labels.createLabel })}
        </p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table
            className="w-full min-w-[40rem] border-collapse"
            aria-label={t('servers.tableLabel', { servers: labels.servers })}
          >
            <thead className="border-b border-border bg-muted/30">
              <tr>
                <th scope="col" className={TH}>
                  {t('servers.name')}
                </th>
                <th scope="col" className={TH}>
                  {t('servers.state')}
                </th>
                <th scope="col" className={TH}>
                  {labels.location}
                </th>
                <th scope="col" className={TH}>
                  {labels.size}
                </th>
                <th scope="col" className={TH}>
                  {t('servers.ip')}
                </th>
                <th scope="col" className={TH}>
                  {t('servers.node')}
                </th>
                <th scope="col" className={TH}>
                  <span className="sr-only">{t('servers.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map((p) => (
                <tr key={p.id}>
                  <td className={`${TD} font-medium`}>
                    {p.name}
                    <span className="block text-xs font-normal text-muted-foreground">
                      <RelativeTime at={p.created_at} />
                    </span>
                  </td>
                  <td className={TD}>
                    <StatusPill
                      tone={STATUS_TONE[p.status]}
                      label={t(`servers.status.${p.status}`)}
                      size="sm"
                      title={p.failure_reason}
                    />
                  </td>
                  <td className={TD}>{p.region}</td>
                  <td className={TD}>{p.size}</td>
                  <td className={`${TD} font-mono text-xs`}>
                    {p.ip_address ?? ''}
                  </td>
                  <td className={TD}>
                    {p.node_id ? (
                      <Link
                        to="/nodes/$id"
                        params={{ id: p.node_id }}
                        aria-label={t('servers.openNode', { name: p.name })}
                        className="underline-offset-2 hover:underline"
                      >
                        {p.name}
                      </Link>
                    ) : (
                      <span className="text-xs text-muted-foreground">
                        {t('servers.noNode')}
                      </span>
                    )}
                  </td>
                  <td className={`${TD} text-end`}>
                    {p.node_id ? (
                      <DestroyServerAction nodeId={p.node_id} />
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
