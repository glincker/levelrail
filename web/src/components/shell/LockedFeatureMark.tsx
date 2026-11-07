import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'

// The word badge already says Locked, so no extra icon: that space keeps the
// label readable. Trailing mark for a gated nav entry that is switched off. The caller points
// aria-describedby at `id` so assistive tech hears why the entry is muted.
export function LockedFeatureMark({ id }: { id: string }) {
  const { t } = useTranslation()
  return (
    <>
      <Badge
        variant="muted"
        className="ml-auto shrink-0 whitespace-nowrap text-sidebar-foreground/80"
      >
        {t('shell.locked.badge')}
      </Badge>
      <span id={id} className="sr-only">
        {t('shell.locked.description')}
      </span>
    </>
  )
}
