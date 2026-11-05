import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import type { DnsTokenVerification } from '../../queries/dnsVerification'

// Shows what the saved credential can do. Hidden when the server sent no
// verification (older server, or verify-on-save turned off).
export function DnsVerificationCallout({
  verification,
}: {
  verification?: DnsTokenVerification
}) {
  const { t } = useTranslation('dns')
  if (!verification) return null
  const missing = verification.missing_scopes ?? []
  if (verification.ok) {
    return (
      <Alert role="status">
        <ShieldCheckGlassIcon
          className="size-5 text-tone-success"
          aria-hidden="true"
        />
        <AlertDescription>
          {t('verification.ok', { count: verification.zones_visible ?? 0 })}
        </AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert variant="destructive" role="alert">
      <CircleAlertGlassIcon className="size-5" aria-hidden="true" />
      <AlertDescription className="space-y-1">
        <p className="font-medium">{t('verification.failedTitle')}</p>
        {missing.length > 0 ? (
          <>
            <p>{t('verification.missing')}</p>
            <ul className="list-disc pl-5 font-mono text-xs">
              {missing.map((scope) => (
                <li key={scope}>{scope}</li>
              ))}
            </ul>
          </>
        ) : null}
        {verification.message ? <p>{verification.message}</p> : null}
        <p>{t('verification.fix')}</p>
      </AlertDescription>
    </Alert>
  )
}
