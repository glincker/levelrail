import { useTranslation } from 'react-i18next'
import { isInsecureRemoteConnection } from '../../lib/connection'
import {
  insecureConnectionItem,
  summarizeAttention,
  type AttentionSummary,
} from '../../lib/attentionSummary'
import { useAttentionItems } from '../../queries/attention'
import { useSnoozed } from './snoozeStore'

// Header chip and dashboard status pill both read this, so they cannot disagree.
export function useAttentionSummary(): AttentionSummary & {
  isLoading: boolean
} {
  const { items, isLoading } = useAttentionItems()
  const { isSnoozed } = useSnoozed()
  const { t } = useTranslation()
  const all = isInsecureRemoteConnection(window.location)
    ? [
        ...items,
        insecureConnectionItem(
          t('shell.status.insecureTitle'),
          t('shell.status.insecureDetail'),
        ),
      ]
    : items
  return { ...summarizeAttention(all, isSnoozed), isLoading }
}
