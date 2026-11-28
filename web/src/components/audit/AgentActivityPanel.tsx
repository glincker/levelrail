import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { BotGlassIcon } from '@thesvg/ui-react'
import { Badge } from '../ui/badge'
import { EmptyState } from '../ui/empty-state'
import { TableSkeleton } from '../ui/table-skeleton'
import { fetchAuditLog, type AuditLogEntry } from '../../queries/auditLog'
import { fetchDeployApprovals } from '../../queries/deployApprovals'
import { approvalIdOf, isNonHumanEntry } from '../../lib/agentActivity'
import { auditFriendlyLabel } from '../../lib/auditLabels'
import type { DeployApprovalResource } from '../../types/deployApproval'

const AGENT_ACTIVITY_LIMIT = 200
function ApprovalCell({ approval }: { approval?: DeployApprovalResource }) {
  const { t } = useTranslation('auditLog')
  if (!approval) {
    return (
      <span className="text-muted-foreground">
        {t('agentActivity.noApproval')}
      </span>
    )
  }
  return (
    <Link
      to="/approvals"
      className="text-foreground underline-offset-2 hover:underline"
    >
      {approval.approved_by_name
        ? t('agentActivity.approvedBy', { name: approval.approved_by_name })
        : t(`agentActivity.status.${approval.status}`)}
    </Link>
  )
}

function ActivityRow({
  entry,
  approvals,
}: {
  entry: AuditLogEntry
  approvals: Map<string, DeployApprovalResource>
}) {
  const { t } = useTranslation('auditLog')
  const friendly = auditFriendlyLabel(entry)
  const id = approvalIdOf(entry)
  const summary = friendly
    ? t(friendly.labelKey)
    : t('agentActivity.summary', { method: entry.method, path: entry.path })
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm">
      <BotGlassIcon
        className="size-5 text-muted-foreground"
        aria-hidden="true"
      />
      <span className="font-medium text-foreground">
        {entry.agent_name || entry.actor_name}
      </span>
      <Badge variant="outline">{entry.client_kind}</Badge>
      <span className="min-w-0 flex-1 truncate" title={entry.path}>
        {summary}
      </span>
      <Badge variant={entry.status_code < 400 ? 'success' : 'destructive'}>
        {entry.status_code}
      </Badge>
      <ApprovalCell approval={id ? approvals.get(id) : undefined} />
      <time className="text-muted-foreground" dateTime={entry.created_at}>
        {new Date(entry.created_at).toLocaleString()}
      </time>
    </li>
  )
}

export default function AgentActivityPanel() {
  const { t } = useTranslation('auditLog')
  const log = useQuery({
    queryKey: ['audit-log', 'agent-activity'],
    queryFn: () => fetchAuditLog({ limit: AGENT_ACTIVITY_LIMIT }),
  })
  const approvalsQuery = useQuery({
    queryKey: ['deploy-approvals', 'agent-activity'],
    queryFn: () => fetchDeployApprovals('all'),
    retry: false,
  })
  const rows = useMemo(
    () => (log.data ?? []).filter(isNonHumanEntry),
    [log.data],
  )
  const approvals = useMemo(
    () => new Map((approvalsQuery.data ?? []).map((a) => [a.id, a])),
    [approvalsQuery.data],
  )
  if (log.isPending) return <TableSkeleton columnCount={6} rowCount={6} />
  if (rows.length === 0) {
    return (
      <EmptyState
        className="gap-2 border-0 bg-transparent py-4"
        icon={<BotGlassIcon className="size-6" />}
        title={t('agentActivity.emptyTitle')}
        description={t('agentActivity.emptyDescription')}
      />
    )
  }
  return (
    <ul
      className="divide-y divide-border"
      aria-label={t('agentActivity.label')}
    >
      {rows.map((e) => (
        <ActivityRow key={e.id} entry={e} approvals={approvals} />
      ))}
    </ul>
  )
}
