import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'

/** RuntimeBadges shows the engine kind and support level the container_runtime check detected. */
export function RuntimeBadges({
  kind,
  level,
}: {
  kind?: string
  level?: string
}) {
  const { t } = useTranslation('fixit')
  if (!kind && !level) return null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {kind ? (
        <Badge variant="muted">
          {t('runtime.engine')}:{' '}
          {t(`runtime.kind.${kind}`, { defaultValue: kind })}
        </Badge>
      ) : null}
      {level ? (
        <Badge variant={level === 'supported' ? 'success' : 'warning'}>
          {t('runtime.support')}:{' '}
          {t(`runtime.level.${level}`, { defaultValue: level })}
        </Badge>
      ) : null}
    </div>
  )
}
