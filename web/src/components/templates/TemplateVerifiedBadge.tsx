import { useTranslation } from 'react-i18next'
import { SealCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

/** TemplateVerifiedBadge marks a template that passed the last live smoke run. */
export function TemplateVerifiedBadge({ className }: { className?: string }) {
  const { t } = useTranslation('templates')
  return (
    <Badge
      variant="muted"
      className={cn('gap-1 text-primary', className)}
      title={t('verified.hint')}
    >
      <SealCheckIcon className="size-3" aria-hidden="true" />
      {t('verified.badge')}
    </Badge>
  )
}
