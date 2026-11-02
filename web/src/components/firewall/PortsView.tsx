import { ShieldGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useExposure } from '../../queries/exposure'
import type { ExposureClass } from '../../types/exposure'

const CLASS_VARIANT: Record<
  ExposureClass,
  'destructive' | 'warning' | 'success' | 'muted'
> = {
  exposed: 'destructive',
  unknown: 'warning',
  restricted: 'success',
  private: 'muted',
  loopback: 'muted',
}

/** PortsView lists published ports per app and whether the internet can reach them; changes are made in Settings. */
export function PortsView() {
  const { t } = useTranslation('exposure')
  const { t: tf } = useTranslation('firewall')
  const { data, isPending, isError } = useExposure(false)

  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{tf('ports.loading')}</p>
    )
  }
  if (isError || !data) {
    return <p className="text-sm text-destructive">{tf('ports.loadError')}</p>
  }
  const rows = data.nodes.flatMap((n) =>
    n.findings.map((f) => ({ node: n, finding: f })),
  )
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={<ShieldGlassIcon className="size-6" aria-hidden="true" />}
        className="gap-2 border-0 bg-transparent py-6"
        title={t('emptyTitle')}
        description={t('emptyBody')}
      />
    )
  }
  return (
    <div
      role="region"
      aria-label={t('tableLabel')}
      tabIndex={0}
      className="overflow-x-auto rounded-lg focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('columns.owner')}</TableHead>
            <TableHead>{t('columns.port')}</TableHead>
            <TableHead>{t('columns.bind')}</TableHead>
            <TableHead>{tf('ports.reach')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(({ node, finding }) => (
            <TableRow
              key={`${node.node_id}:${finding.protocol}:${finding.host_port}:${finding.container}`}
            >
              <TableCell className="text-xs">
                {t(`owner.${finding.owner.kind}`, { name: finding.owner.name })}
                <span className="block text-muted-foreground">
                  {finding.container}
                </span>
              </TableCell>
              <TableCell className="font-mono text-xs">
                {finding.host_port}/{finding.protocol}
              </TableCell>
              <TableCell className="font-mono text-xs">
                {finding.binds.join(', ')}
              </TableCell>
              <TableCell>
                <Badge variant={CLASS_VARIANT[finding.class]}>
                  {t(`class.${finding.class}`)}
                </Badge>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
