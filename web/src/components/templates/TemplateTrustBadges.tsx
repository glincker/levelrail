import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  PinGlassIcon,
  TagGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { summarizeImagePins } from '../../lib/templateTrust'

/** Freshness badges derived from image references only; nothing is shown without an image. */
export function TemplateTrustBadges({ images }: { images: string[] }) {
  const { t } = useTranslation('templates')
  const s = summarizeImagePins(images)
  if (s.total === 0) return null
  return (
    <>
      {s.digest > 0 ? (
        <Badge variant="muted" title={t('trust.digestHint')}>
          <PinGlassIcon className="size-4.5" aria-hidden="true" />
          {t('trust.digest', { count: s.digest })}
        </Badge>
      ) : null}
      {s.tag > 0 ? (
        <Badge variant="muted" title={t('trust.tagHint')}>
          <TagGlassIcon className="size-4.5" aria-hidden="true" />
          {t('trust.tag', { count: s.tag })}
        </Badge>
      ) : null}
      {s.floating > 0 ? (
        <Badge variant="outline" title={t('trust.floatingHint')}>
          <CircleAlertGlassIcon
            className="size-4.5 text-tone-danger"
            aria-hidden="true"
          />
          {t('trust.floating', { count: s.floating })}
        </Badge>
      ) : null}
    </>
  )
}
