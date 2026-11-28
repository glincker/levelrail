import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatDate, formatSize } from '../../lib/format'
import { useNodeListOptional } from '../../queries/nodes'
import { useNodeCleanupRuns } from '../../queries/nodeCapacity'
import { doctorQueryOptions } from '../../queries/releases'

const DRY_TRIGGER = 'janitor-dry'
const DISK_BUDGET_CHECK = 'node_disk_budget'

function NodeRow({ id, name }: Readonly<{ id: string; name: string }>) {
  const { t } = useTranslation('janitor')
  const runs = useNodeCleanupRuns(id)
  const last = runs.data?.find((r) => r.trigger === DRY_TRIGGER)
  const candidates = last
    ? last.classes.reduce((sum, c) => sum + c.bytes, 0)
    : 0
  return (
    <li className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
      <span className="font-medium">{name}</span>
      <span className="text-muted-foreground">
        {runs.isPending
          ? t('remote.loading')
          : last
            ? t('remote.lastDry', {
                when: formatDate(last.started_at, ''),
                size: formatSize(candidates),
              })
            : t('remote.noDry')}
      </span>
    </li>
  )
}

/** Remote nodes: last preview pass per node and the disk budget finding. */
export default function RemoteNodesCard() {
  const { t } = useTranslation('janitor')
  const nodes = useNodeListOptional()
  const doctor = useQuery({
    ...doctorQueryOptions(),
    queryKey: ['system', 'doctor', 'janitor-remote'],
    enabled: true,
    retry: false,
  })
  const finding = doctor.data?.checks.find((c) => c.code === DISK_BUDGET_CHECK)
  const list = nodes.data ?? []

  return (
    <Card data-testid="remote-nodes-card">
      <CardHeader>
        <CardTitle>{t('remote.title')}</CardTitle>
        <CardDescription>{t('remote.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {finding ? (
          <p className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={finding.status === 'ok' ? 'success' : 'warning'}>
              {t(`remote.status.${finding.status}`)}
            </Badge>
            <span data-testid="disk-budget-message">{finding.message}</span>
          </p>
        ) : null}
        {list.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('remote.empty')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {list.map((n) => (
              <NodeRow key={n.id} id={n.id} name={n.name} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
