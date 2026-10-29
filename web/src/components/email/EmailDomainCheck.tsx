import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  useEmailDomainCheck,
  type EmailRecordCheck,
} from '../../queries/emailSettings'
import { PRESS } from './emailStyles'

const PROVIDER_HINT_KEYS = new Set([
  'ses',
  'ses_smtp',
  'resend',
  'postmark',
  'sendgrid',
  'mailgun',
])

function RecordRow({
  record,
  include,
}: {
  record: EmailRecordCheck
  include?: string
}) {
  const { t } = useTranslation('settings')
  const pass = record.status === 'pass'
  return (
    <li className="space-y-1 rounded-md border border-border bg-background/40 p-2">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={pass ? 'success' : 'warning'}>
          {pass ? (
            <CircleCheckGlassIcon
              className="size-3.5 text-tone-success"
              aria-hidden="true"
            />
          ) : (
            <CircleAlertGlassIcon
              className="size-3.5 text-tone-danger"
              aria-hidden="true"
            />
          )}
          {pass
            ? t('emailDelivery.domain.pass')
            : t('emailDelivery.domain.warn')}
        </Badge>
        <span className="text-sm font-medium">
          {t(`emailDelivery.domain.types.${record.type}`)}
        </span>
        <span className="font-mono text-xs break-all text-muted-foreground">
          {record.name}
        </span>
      </div>
      <p className="text-sm">
        {t(`emailDelivery.domain.codes.${record.code as 'spf_ok'}`, {
          include: include ?? '',
        })}
      </p>
      {record.value ? (
        <p className="font-mono text-xs break-all text-muted-foreground">
          {record.value}
        </p>
      ) : null}
    </li>
  )
}

/** DomainCheckInline looks up SPF, DKIM and DMARC for one provider's sender domain through the control plane. */
export function DomainCheckInline({
  providerId,
  backend,
}: Readonly<{ providerId?: string; backend: string }>) {
  const { t } = useTranslation('settings')
  const check = useEmailDomainCheck()
  const data = check.data

  return (
    <div className="space-y-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        className={PRESS}
        disabled={check.isPending}
        onClick={() => {
          check.mutate(providerId)
        }}
      >
        <ShieldCheckGlassIcon
          className="text-tone-success"
          size="1.2em"
          aria-hidden="true"
        />
        {check.isPending
          ? t('emailDelivery.domain.checking')
          : data
            ? t('emailDelivery.domain.recheck')
            : t('emailDelivery.domain.run')}
      </Button>
      <div aria-live="polite" className="space-y-2 empty:hidden">
        {check.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{check.error.message}</AlertDescription>
          </Alert>
        ) : null}
        {data ? (
          <>
            <p className="text-xs text-muted-foreground">
              {t('emailDelivery.domain.checked', { domain: data.domain })}
            </p>
            <ul className="space-y-1.5">
              {data.records.map((r) => (
                <RecordRow key={r.type} record={r} include={data.spf_include} />
              ))}
            </ul>
            {PROVIDER_HINT_KEYS.has(backend) ? (
              <p className="text-xs text-muted-foreground">
                {t(`emailDelivery.domain.providerHints.${backend as 'ses'}`)}
              </p>
            ) : null}
          </>
        ) : null}
      </div>
    </div>
  )
}
