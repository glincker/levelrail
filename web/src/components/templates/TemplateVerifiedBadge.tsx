import { useTranslation } from 'react-i18next'
import { BadgeCheckGlassIcon } from '@thesvg/ui-react'
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
      <BadgeCheckGlassIcon
        className="size-3.5 text-tone-success"
        aria-hidden="true"
      />
      {t('verified.badge')}
    </Badge>
  )
}
