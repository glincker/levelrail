import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { formatBytes } from '../../lib/format'
import type { BuildImageBloat } from '../../queries/buildCenter'

// Warns when a build's image grew past the bloat threshold, naming the layer that grew.
export function ImageBloatNote({
  bloat,
}: Readonly<{ bloat?: BuildImageBloat | null }>) {
  const { t } = useTranslation('imageBloat')
  if (!bloat) return null
  const percent = Math.round(bloat.growth_percent)
  const layer = bloat.layer
    ? t('layer', {
        layer: bloat.layer,
        size: formatBytes(bloat.layer_bytes ?? 0),
      })
    : ''
  const detail = t('detail', {
    percent,
    previous: formatBytes(bloat.previous_bytes),
  })
  return (
    <span
      className="flex max-w-full items-center gap-1 text-xs text-tone-warning"
      title={[detail, layer, t('tuneHint')].filter(Boolean).join(' ')}
      data-testid="image-bloat"
    >
      <TriangleAlertGlassIcon className="size-4 shrink-0" aria-hidden="true" />
      <span className="truncate">{t('badge', { percent })}</span>
      {layer ? <span className="sr-only">{layer}</span> : null}
    </span>
  )
}
