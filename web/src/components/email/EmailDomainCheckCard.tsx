import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  ShieldCheckIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  useEmailDomainCheck,
  type EmailRecordCheck,
} from '../../queries/emailSettings'

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
    <li className="space-y-1 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={pass ? 'success' : 'warning'}>
          {pass ? (
            <CheckCircleIcon className="size-3" aria-hidden="true" />
          ) : (
            <WarningCircleIcon className="size-3" aria-hidden="true" />
          )}
          {pass
            ? t('emailDelivery.domain.pass')
            : t('emailDelivery.domain.warn')}
        </Badge>
        <span className="text-sm font-medium">
          {t(`emailDelivery.domain.types.${record.type}`)}
        </span>
        <span className="font-mono text-xs text-muted-foreground break-all">
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

/** EmailDomainCheckCard looks up SPF, DKIM and DMARC for the From domain through the control plane. */
export function EmailDomainCheckCard({ provider }: { provider: string }) {
  const { t } = useTranslation('settings')
  const check = useEmailDomainCheck()
  const data = check.data

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="min-w-0 flex-1 space-y-0.5">
            <div className="flex items-center gap-2 text-sm font-medium">
              <ShieldCheckIcon className="size-4" aria-hidden="true" />
              {t('emailDelivery.domain.title')}
            </div>
            <p className="text-sm text-muted-foreground">
              {t('emailDelivery.domain.description')}
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={check.isPending}
            onClick={() => check.mutate()}
          >
            {check.isPending
              ? t('emailDelivery.domain.checking')
              : data
                ? t('emailDelivery.domain.recheck')
                : t('emailDelivery.domain.run')}
          </Button>
        </div>
        {check.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{check.error.message}</AlertDescription>
          </Alert>
        ) : null}
        {data ? (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {t('emailDelivery.domain.checked', { domain: data.domain })}
            </p>
            <ul className="space-y-2">
              {data.records.map((r) => (
                <RecordRow key={r.type} record={r} include={data.spf_include} />
              ))}
            </ul>
            {PROVIDER_HINT_KEYS.has(provider) ? (
              <p className="text-sm text-muted-foreground">
                {t(`emailDelivery.domain.providerHints.${provider as 'ses'}`)}
              </p>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
