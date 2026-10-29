import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'

// Mounted on app and database pages by the slice that knows their `managed_by` field.
export function ManagedByStackBadge({ stack }: Readonly<{ stack: string }>) {
  const { t } = useTranslation('stacks')
  return (
    <Link
      to="/settings/stacks"
      search={{ stack }}
      aria-label={t('badge.managedBy', { name: stack })}
      className="inline-flex"
    >
      <Badge variant="info">
        <LayersGlassIcon size="1.2em" aria-hidden="true" />
        {t('badge.short', { name: stack })}
      </Badge>
    </Link>
  )
}
