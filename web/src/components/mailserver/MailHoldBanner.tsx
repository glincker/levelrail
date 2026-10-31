import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'

/** MailHoldBanner tells the operator a restore holds the mail app stopped. */
export function MailHoldBanner({ hold }: { hold: string }) {
  const { t } = useTranslation('mailServer')
  return (
    <div
      role="status"
      className="flex items-start gap-3 rounded-lg border border-amber-500/60 bg-amber-500/10 px-3 py-2"
    >
      <LockGlassIcon size="1.3em" className="mt-0.5 shrink-0" />
      <div className="min-w-0 space-y-0.5">
        <p className="text-sm font-semibold">{t('hold.title')}</p>
        <p className="text-sm text-muted-foreground">{t('hold.body')}</p>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {hold}
        </p>
      </div>
    </div>
  )
}
