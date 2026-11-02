import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  domainCutoverQueryOptions,
  useCutoverCertificate,
  useRevertCutoverChallenge,
  type CutoverState,
  type CutoverStep,
  type DomainCutover,
} from '../../queries/domainCutover'

const STATE_ICON: Record<CutoverState, { icon: UiIcon; className: string }> = {
  pass: { icon: CircleCheckGlassIcon, className: 'text-emerald-600' },
  wait: { icon: ClockGlassIcon, className: 'text-tone-warning' },
  fail: { icon: CircleXGlassIcon, className: 'text-destructive' },
  skip: { icon: CircleMinusGlassIcon, className: 'text-muted-foreground' },
}

function etaMinutes(seconds: number): number {
  return Math.max(1, Math.ceil(seconds / 60))
}

function Step({
  step,
  index,
  data,
}: {
  step: CutoverStep
  index: number
  data: DomainCutover
}) {
  const { t } = useTranslation('domains')
  const cert = useCutoverCertificate(data.domain)
  const revert = useRevertCutoverChallenge(data.domain)
  const { icon: Icon, className } = STATE_ICON[step.state]
  const canPreIssue =
    step.id === 'certificate' &&
    step.action?.kind === 'request' &&
    step.action.api?.startsWith('POST') === true &&
    step.state !== 'pass'
  const canRevert =
    step.id === 'certificate' && step.action?.api?.startsWith('DELETE') === true
  return (
    <li className="flex gap-2 py-2">
      <Icon
        size="1.2em"
        className={`mt-0.5 shrink-0 ${className}`}
        aria-hidden
      />
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium">
          {index + 1}. {t(`cutover.steps.${step.id}`)}{' '}
          <span className="sr-only">{t(`cutover.state.${step.state}`)}</span>
          {step.eta_seconds && step.state === 'wait' ? (
            <span className="ml-1 text-xs font-normal text-muted-foreground">
              {t('cutover.eta', { minutes: etaMinutes(step.eta_seconds) })}
            </span>
          ) : null}
        </p>
        <p className="text-xs text-muted-foreground">{step.detail}</p>
        {step.id === 'switch_dns' && step.state === 'wait'
          ? (data.dns.records ?? []).map((r) => (
              <p
                key={`${r.type}-${r.value}`}
                className="font-mono text-xs break-all"
              >
                {r.type} {r.name} {r.value} (TTL {r.ttl_seconds})
              </p>
            ))
          : null}
        {canPreIssue ? (
          <Button
            type="button"
            size="sm"
            disabled={cert.isPending}
            onClick={() => {
              cert.mutate()
            }}
          >
            {t('cutover.getCertificate')}
          </Button>
        ) : null}
        {canRevert ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={revert.isPending}
            onClick={() => {
              revert.mutate()
            }}
          >
            {t('cutover.revertChallenge')}
          </Button>
        ) : null}
        {step.id === 'certificate' && data.certificate.challenge === 'dns01' ? (
          <p className="text-xs text-muted-foreground">
            {t('cutover.dns01ThisDomain')}
          </p>
        ) : null}
        {step.id === 'certificate' && (cert.error ?? revert.error) ? (
          <p className="text-xs text-destructive">
            {(cert.error ?? revert.error)?.message}
          </p>
        ) : null}
        {step.id === 'certificate' &&
        step.state !== 'pass' &&
        step.state !== 'skip' &&
        !data.certificate.provider_connected ? (
          <p className="text-xs text-muted-foreground">
            {t('cutover.noProvider', { ttl: data.dns.lower_ttl_to })}{' '}
            <a className="underline" href={data.certificate.runbook_href}>
              {t('cutover.runbook')}
            </a>
          </p>
        ) : null}
      </div>
    </li>
  )
}

export function CutoverChecklist({ data }: { data: DomainCutover }) {
  const { t } = useTranslation('domains')
  const certPending = data.steps.find((s) => s.id === 'certificate')?.state
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge
          variant={
            data.risk === 'none'
              ? 'success'
              : data.risk === 'likely'
                ? 'destructive'
                : 'muted'
          }
        >
          {t(`cutover.risk.${data.risk}`)}
        </Badge>
        <p className="text-sm">
          {data.ready_to_switch
            ? t('cutover.ready', {
                domain: data.domain,
                cert:
                  certPending === 'pass' || certPending === 'skip'
                    ? t('cutover.certReady')
                    : t('cutover.certPending'),
              })
            : t('cutover.notReady', { domain: data.domain })}
        </p>
      </div>
      {data.proxied ? (
        <Alert>
          <CircleAlertGlassIcon size="1.2em" aria-hidden />
          <AlertDescription>
            {t('cutover.proxied')}{' '}
            <a
              className="underline"
              href={data.proxy_docs ?? '/cdn-and-caching'}
            >
              {t('cutover.proxyDocs')}
            </a>
          </AlertDescription>
        </Alert>
      ) : null}
      <ol className="divide-y divide-border">
        {data.steps.map((s, i) => (
          <Step key={s.id} step={s} index={i} data={data} />
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">
        <a className="underline" href={data.guide_href}>
          {t('cutover.guide')}
        </a>
      </p>
    </div>
  )
}

export function DomainCutoverCard({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const { data, isPending, error } = useQuery(domainCutoverQueryOptions(domain))
  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('cutover.loading')}</p>
    )
  }
  if (error || !data) {
    return <p className="text-sm text-destructive">{t('cutover.error')}</p>
  }
  return <CutoverChecklist data={data} />
}
