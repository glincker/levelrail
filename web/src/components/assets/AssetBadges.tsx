import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon, LockGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type { AssetBucket } from '../../types/assets'

export function VisibilityBadge({
  visibility,
}: Readonly<{ visibility: AssetBucket['visibility'] }>) {
  const { t } = useTranslation('assets')
  return visibility === 'public' ? (
    <Badge variant="warning">
      <GlobeGlassIcon size="1.4em" aria-hidden="true" />
      {t('visibility.public')}
    </Badge>
  ) : (
    <Badge variant="muted">
      <LockGlassIcon size="1.4em" aria-hidden="true" />
      {t('visibility.private')}
    </Badge>
  )
}

export function ServingBadge({ mode }: Readonly<{ mode: string }>) {
  const { t } = useTranslation('assets')
  return <Badge variant="outline">{t(`serving.${mode}`, mode)}</Badge>
}

export function PresetLabel({ preset }: Readonly<{ preset?: string }>) {
  const { t } = useTranslation('assets')
  const key = preset ?? 'custom'
  return <span>{t(`preset.${key}`, key)}</span>
}
