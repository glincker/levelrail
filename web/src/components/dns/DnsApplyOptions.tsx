import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

/** Approve at the DNS host (no token) next to connect with a token; the approve card exists only when the server offers the flow. */
export function DnsApplyOptions({
  host,
  flow,
  token,
  approving,
  certs,
  mail,
  onApprove,
}: {
  host: string
  /** domain_connect.flow from detection; false renders no approve card. */
  flow: boolean
  /** The connect-with-token trigger (opens the token popover). */
  token: ReactNode
  approving: boolean
  certs: boolean
  mail: boolean
  onApprove: () => void
}) {
  const { t } = useTranslation('dnsSetup')
  return (
    <div
      role="group"
      aria-label={t('options.label')}
      className="grid gap-2 sm:grid-cols-2"
      data-testid="apply-options"
    >
      {flow && (
        <div className="flex flex-col items-start gap-1.5 rounded-md border border-border p-2">
          <Button
            type="button"
            size="sm"
            disabled={approving}
            onClick={onApprove}
          >
            {approving
              ? t('options.approving')
              : t('options.approve', { host })}
          </Button>
          <p className="text-xs text-muted-foreground">
            {t('options.approveHint')}
          </p>
        </div>
      )}
      <div className="flex flex-col items-start gap-1.5 rounded-md border border-border p-2">
        {token}
        <p className="text-xs text-muted-foreground">
          {t('options.tokenHint')}
          {certs && mail ? ` ${t('options.tokenCerts')}` : ''}
        </p>
      </div>
    </div>
  )
}
