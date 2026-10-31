import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useResetMailDemo } from '@/queries/mailDemo'

/** MailDemoBadge marks the Mail page as showing fake data from local demo mode. */
export function MailDemoBadge() {
  const { t } = useTranslation('mailServer')
  const reset = useResetMailDemo()
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/60 bg-amber-500/10 px-3 py-2"
    >
      <span className="rounded bg-amber-500 px-2 py-0.5 text-xs font-bold tracking-wider text-black">
        {t('demo.badge')}
      </span>
      <span className="text-sm text-muted-foreground">{t('demo.hint')}</span>
      <Button
        size="sm"
        variant="outline"
        className="ml-auto"
        disabled={reset.isPending}
        onClick={() => reset.mutate()}
      >
        {t('demo.reset')}
      </Button>
    </div>
  )
}
