import { useTranslation } from 'react-i18next'
import { LockSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'

// Trailing mark for a gated nav entry that is switched off. The caller points
// aria-describedby at `id` so assistive tech hears why the entry is muted.
export function LockedFeatureMark({ id }: { id: string }) {
  const { t } = useTranslation()
  return (
    <>
      <LockSimpleIcon
        className="ml-auto size-3.5 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <Badge variant="muted" className="shrink-0 whitespace-nowrap">
        {t('shell.locked.badge')}
      </Badge>
      <span id={id} className="sr-only">
        {t('shell.locked.description')}
      </span>
    </>
  )
}
