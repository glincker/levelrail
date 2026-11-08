import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { Alert } from '@/components/ui/alert'

/** MailHoldBanner tells the operator a restore holds the mail app stopped. */
export function MailHoldBanner({ hold }: { hold: string }) {
  const { t } = useTranslation('mailServer')
  return (
    <Alert
      variant="warning"
      role="status"
      className="flex items-start gap-3 px-3 py-2"
    >
      <LockGlassIcon
        size="1.3em"
        className="mt-0.5 shrink-0"
        aria-hidden="true"
      />
      <div className="min-w-0 space-y-0.5">
        <p className="text-sm font-semibold">{t('hold.title')}</p>
        <p className="text-sm text-muted-foreground">{t('hold.body')}</p>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {hold}
        </p>
      </div>
    </Alert>
  )
}
