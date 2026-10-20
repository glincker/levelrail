import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { TeamRole } from '../../queries/teams'

export function TeamRoleBadge({ role }: { role: TeamRole }) {
  const { t } = useTranslation('teams')
  return (
    <Badge
      variant={role === 'viewer' ? 'muted' : 'outline'}
      title={t(`roles.${role}Hint`)}
      aria-label={t('roles.yourRole', { role: t(`roles.${role}`) })}
    >
      {t(`roles.${role}`)}
    </Badge>
  )
}
