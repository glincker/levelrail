import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type {
  TakeoverCheck,
  TakeoverDomain,
  TakeoverPlanStep,
} from '../queries/portTakeover'

const checkVariant = {
  pass: 'success',
  warn: 'warning',
  block: 'destructive',
} as const

function FixAction({ check }: { check: TakeoverCheck }) {
  const { t } = useTranslation('migration')
  const [copied, setCopied] = useState(false)
  const action = check.fix?.action
  if (!action || check.status === 'pass') return null
  if (action.kind === 'copy' && action.value) {
    const value = action.value
    return (
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          void navigator.clipboard.writeText(value)
          setCopied(true)
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" />
        ) : (
          <CopyGlassIcon size="1.2em" />
        )}
        {copied ? t('takeover.copied') : t('takeover.copyCommand')}
      </Button>
    )
  }
  if (action.kind === 'link' && action.value) {
    return (
      <Button
        size="sm"
        variant="outline"
        render={<a href={action.value}>{action.label}</a>}
        nativeButton={false}
      />
    )
  }
  return null
}

export function CheckRow({ check }: { check: TakeoverCheck }) {
  const { t } = useTranslation('migration')
  return (
    <li className="flex flex-wrap items-start gap-2 text-sm">
      <Badge variant={checkVariant[check.status]}>
        {t(`takeover.status.${check.status}`)}
      </Badge>
      <span className="min-w-0 flex-1">
        <span className="font-medium">{check.title}</span>
        {check.detail ? (
          <span className="block text-muted-foreground">{check.detail}</span>
        ) : null}
        {check.items && check.items.length > 0 ? (
          <ul className="mt-1 list-disc pl-5 text-muted-foreground">
            {check.items.map((item) => (
              <li key={item} className="break-all">
                {item}
              </li>
            ))}
          </ul>
        ) : null}
        {check.status !== 'pass' && check.fix ? (
          <span className="block text-muted-foreground">
            {check.fix.summary}
          </span>
        ) : null}
      </span>
      <FixAction check={check} />
    </li>
  )
}

export function DomainList({ domains }: { domains: TakeoverDomain[] }) {
  const { t } = useTranslation('migration')
  if (domains.length === 0) return null
  return (
    <section className="space-y-1">
      <h4 className="text-sm font-semibold">{t('takeover.domainsTitle')}</h4>
      <ul className="space-y-1 text-sm">
        {domains.map((d) => (
          <li key={d.domain} className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs break-all">{d.domain}</span>
            <span className="text-muted-foreground">{d.app}</span>
            <Badge variant={d.healthy ? 'success' : 'destructive'}>
              {d.healthy ? t('takeover.healthy') : t('takeover.unhealthy')}
            </Badge>
            <Badge variant={d.dns === 'ok' ? 'success' : 'warning'}>
              {t(`takeover.dns.${d.dns}`)}
            </Badge>
            <Badge variant="muted">{t(`takeover.cert.${d.certificate}`)}</Badge>
          </li>
        ))}
      </ul>
    </section>
  )
}

export function PlanSteps({
  steps,
  rollback,
}: {
  steps: TakeoverPlanStep[]
  rollback: string[]
}) {
  const { t } = useTranslation('migration')
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <section className="space-y-1">
        <h4 className="text-sm font-semibold">{t('takeover.stepsTitle')}</h4>
        <ol className="list-decimal space-y-1 pl-5 text-sm">
          {steps.map((s) => (
            <li key={s.id}>
              <span className="font-medium">{s.title}</span>
              <span className="block text-muted-foreground">{s.detail}</span>
            </li>
          ))}
        </ol>
      </section>
      <section className="space-y-1">
        <h4 className="text-sm font-semibold">{t('takeover.rollbackTitle')}</h4>
        <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
          {rollback.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </section>
    </div>
  )
}
