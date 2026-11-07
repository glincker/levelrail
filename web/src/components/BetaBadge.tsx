import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

// Marks a surface that works but is not tuned for large databases or heavy
// load yet. The hint is the native title so it reaches keyboard and touch
// users without a tooltip trigger.
export function BetaBadge({
  className,
  hint,
}: {
  className?: string
  hint?: string
}) {
  const { t } = useTranslation('databases')
  return (
    <span
      className={cn('text-xs font-medium text-muted-foreground', className)}
      title={hint ?? t('viewer.beta.hint')}
      data-testid="beta-badge"
    >
      {t('viewer.beta.label')}
    </span>
  )
}
