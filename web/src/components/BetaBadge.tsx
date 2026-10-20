import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'

// Marks a surface that works but is not tuned for large databases or heavy
// load yet. The hint is the native title so it reaches keyboard and touch
// users without a tooltip trigger.
export function BetaBadge({ className }: { className?: string }) {
  const { t } = useTranslation('databases')
  return (
    <Badge
      variant="outline"
      className={className}
      title={t('viewer.beta.hint')}
      data-testid="beta-badge"
    >
      {t('viewer.beta.label')}
    </Badge>
  )
}
