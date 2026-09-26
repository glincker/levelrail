import { useAttentionItems } from '../../queries/attention'
import { useDeployApprovalsOptional } from '../../queries/deployApprovals'

export function useNavCounts(): Record<string, number> {
  const approvals = useDeployApprovalsOptional('pending')
  const attention = useAttentionItems().items
  return {
    approvals: approvals.data?.length ?? 0,
    'failing-apps': attention.filter((i) => i.id.startsWith('app:')).length,
  }
}
