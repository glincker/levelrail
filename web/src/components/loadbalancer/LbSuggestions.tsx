import { LightbulbIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { SuggestionList } from '@/components/kit'
import type { LbSuggestion } from './suggestions'

export function LbSuggestions({
  items,
  pendingId,
  onApply,
  onDismiss,
}: {
  items: LbSuggestion[]
  pendingId: string | null
  onApply: (item: LbSuggestion) => void
  onDismiss: (id: string) => void
}) {
  if (items.length === 0) return null
  return (
    <SuggestionList
      items={items.map((s) => ({
        id: s.id,
        tone: s.tone,
        icon: s.tone === 'warning' ? <WarningIcon /> : <LightbulbIcon />,
        title: s.title,
        detail: s.detail,
        actions: [
          {
            label: s.actionLabel,
            kind: 'primary' as const,
            pending: pendingId === s.id,
            onClick: () => onApply(s),
          },
        ],
        onDismiss: () => onDismiss(s.id),
      }))}
    />
  )
}
