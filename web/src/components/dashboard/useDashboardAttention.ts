import { isSignInItem, withoutItems } from '../../lib/attentionSummary'
import { useAttentionSummary } from '../shell/useAttentionSummary'

// Sign-in requests already show in the global banner, so the strip and next steps leave them out.
export function useDashboardAttention() {
  const summary = useAttentionSummary()
  return {
    ...withoutItems(summary, isSignInItem),
    isLoading: summary.isLoading,
  }
}
