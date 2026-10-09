import { useTranslation } from 'react-i18next'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import type { AcmeFailure } from '../queries/domainCheck'

// Why a certificate is missing or stuck, in the CA's own words, with the
// one concrete step that usually fixes it.
export function AcmeFailureHint({
  failure,
  compact = false,
}: {
  failure: AcmeFailure
  compact?: boolean
}) {
  const { t } = useTranslation('domains')
  return (
    <div
      role="status"
      className="space-y-1 rounded-md border border-destructive/30 bg-destructive/5 p-2 text-xs"
    >
      <p className="flex items-center gap-1.5 font-medium text-foreground">
        <WarningCircleIcon className="size-3.5 shrink-0" aria-hidden="true" />
        {failure.renewal ? t('acme.renewalTitle') : t('acme.title')}
      </p>
      {compact ? null : (
        <p className="break-words text-muted-foreground">
          {t('acme.caError')}:{' '}
          <code className="font-mono">{failure.error}</code>
        </p>
      )}
      <p className="text-foreground">{t(`acme.action.${failure.action}`)}</p>
    </div>
  )
}
