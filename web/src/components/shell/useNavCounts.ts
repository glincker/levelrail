import { inboxUnread } from '../../lib/inboxEntries'
import { useAttentionItems } from '../../queries/attention'
import { useDeployApprovalsOptional } from '../../queries/deployApprovals'
import {
  attentionCounts,
  useTrafficSummary,
} from '../../queries/trafficSummary'
import { useInboxEntries } from '../../queries/inboxFeed'

export function useNavCounts(): Record<string, number> {
  const approvals = useDeployApprovalsOptional('pending')
  const attention = useAttentionItems().items
  const inbox = inboxUnread(useInboxEntries().entries)
  const traffic = attentionCounts(useTrafficSummary().data)
  return {
    approvals: approvals.data?.length ?? 0,
    inbox,
    'failing-apps': attention.filter((i) => i.id.startsWith('app:')).length,
    'domains-attention': traffic.domains,
    'dns-attention': traffic.dns,
    'proxy-attention': traffic.proxy,
  }
}
