import { useState } from 'react'
import { GoLiveLink } from '../golive/GoLiveBadge'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  CopyGlassIcon,
  RefreshCwGlassIcon,
  WrenchGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  useApplyDiagnoseAction,
  useDomainDiagnosis,
  type DiagnoseCheck,
  type DiagnoseFix,
  type DiagnoseRecord,
  type DiagnoseState,
} from '../../queries/domainDiagnose'
import { BrandIcon } from '../BrandIcon'

const STATE_ICON: Record<DiagnoseState, { icon: UiIcon; className: string }> = {
  pass: { icon: CircleCheckGlassIcon, className: 'text-emerald-600' },
  warn: { icon: CircleAlertGlassIcon, className: 'text-tone-warning' },
  fail: { icon: CircleXGlassIcon, className: 'text-destructive' },
  skipped: { icon: CircleMinusGlassIcon, className: 'text-muted-foreground' },
  unavailable: {
    icon: CircleMinusGlassIcon,
    className: 'text-muted-foreground',
  },
}

const NEEDS_FIX: DiagnoseState[] = ['fail', 'warn']

function sortProblems(checks: DiagnoseCheck[]): DiagnoseCheck[] {
  return checks
    .filter((c) => NEEDS_FIX.includes(c.state))
    .sort((a, b) =>
      a.state === b.state ? a.tier - b.tier : a.state === 'fail' ? -1 : 1,
    )
}

function RecordRow({ record }: { record: DiagnoseRecord }) {
  const { t } = useTranslation('domains')
  const { copied, copy } = useCopyToClipboard()
  return (
    <li className="flex items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1.5 font-mono text-xs">
      <span className="w-12 shrink-0 font-semibold">{record.type}</span>
      <span className="min-w-0 flex-1 break-all">{record.name}</span>
      <span className="min-w-0 flex-1 break-all">{record.value}</span>
      <Button
        type="button"
        size="icon-xs"
        variant="ghost"
        aria-label={t('diagnose.copyRecord', {
          type: record.type,
          name: record.name,
        })}
        onClick={() => {
          copy(`${record.name} ${record.type} ${record.value}`)
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" />
        ) : (
          <CopyGlassIcon size="1.2em" />
        )}
      </Button>
    </li>
  )
}

function CliFallback({ command }: { command: string }) {
  const { t } = useTranslation('domains')
  return (
    <Collapsible>
      <CollapsibleTrigger className="text-xs text-muted-foreground underline-offset-2 hover:underline">
        {t('diagnose.fixViaCli')}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <code className="mt-1 block rounded-md bg-muted px-2 py-1 font-mono text-xs break-all">
          {command}
        </code>
      </CollapsiblePanel>
    </Collapsible>
  )
}

function FixBlock({
  fix,
  onApplied,
}: {
  fix: DiagnoseFix
  onApplied: () => void
}) {
  const { t } = useTranslation('domains')
  const apply = useApplyDiagnoseAction()
  const action = fix.action
  const canApply = fix.kind === 'auto' && action?.kind === 'request'
  return (
    <div className="mt-2 space-y-2 border-l-2 border-border pl-3">
      <p className="text-sm">{fix.summary}</p>
      {fix.records && fix.records.length > 0 ? (
        <ul className="space-y-1">
          {fix.records.map((r) => (
            <RecordRow key={`${r.type}-${r.name}-${r.value}`} record={r} />
          ))}
        </ul>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        {canApply ? (
          <Button
            type="button"
            size="sm"
            disabled={apply.isPending}
            onClick={() => {
              apply.mutate(action, {
                onSuccess: () => {
                  toast.add({
                    title: t('diagnose.applied', { label: action.label }),
                    type: 'success',
                  })
                  onApplied()
                },
                onError: (err) => {
                  toast.add({ title: err.message, type: 'error' })
                },
              })
            }}
          >
            <WrenchGlassIcon size="1.2em" />
            {apply.isPending ? t('diagnose.applying') : action.label}
          </Button>
        ) : null}
        {action?.kind === 'link' && action.api ? (
          <Button
            size="sm"
            variant="outline"
            nativeButton={false}
            render={<a href={action.api} />}
          >
            {action.label}
          </Button>
        ) : null}
        <Button type="button" size="sm" variant="outline" onClick={onApplied}>
          <RefreshCwGlassIcon size="1.2em" />
          {t('diagnose.recheck')}
        </Button>
      </div>
      {fix.cli ? <CliFallback command={fix.cli} /> : null}
    </div>
  )
}

function CheckRow({
  check,
  onApplied,
}: {
  check: DiagnoseCheck
  onApplied: () => void
}) {
  const { icon: StateIcon, className } = STATE_ICON[check.state]
  return (
    <li className="py-2">
      <div className="flex items-start gap-2">
        <StateIcon
          className={`mt-0.5 size-4 shrink-0 ${className}`}
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-1.5 text-sm font-medium">
            {check.id === 'dns.proxied' && check.state !== 'pass' ? (
              <BrandIcon name="cloudflare" className="size-4" />
            ) : null}
            <span>{check.title}</span>
            <span className="sr-only">{check.state}</span>
          </p>
          {check.detail ? (
            <p className="text-xs break-words text-muted-foreground">
              {check.detail}
            </p>
          ) : null}
          {check.fix && NEEDS_FIX.includes(check.state) ? (
            <FixBlock fix={check.fix} onApplied={onApplied} />
          ) : null}
        </div>
      </div>
    </li>
  )
}

function PropagationLine({
  state,
  detail,
  seconds,
}: {
  state: 'settled' | 'propagating' | 'waiting'
  detail: string
  seconds?: number
}) {
  const { t } = useTranslation('domains')
  const variant =
    state === 'settled' ? 'success' : state === 'waiting' ? 'muted' : 'warning'
  return (
    <p className="flex flex-wrap items-center gap-2 text-xs" aria-live="polite">
      <Badge variant={variant}>{t(`diagnose.propagation.${state}`)}</Badge>
      <span className="text-muted-foreground">{detail}</span>
      {seconds ? (
        <span className="text-muted-foreground">
          {t('diagnose.nextCheck', { seconds })}
        </span>
      ) : null}
    </p>
  )
}

/** DomainDiagnosePanel runs the domain diagnosis, shows each check as it arrives, and gives every problem its fix and a Re-check. */
export function DomainDiagnosePanel({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
  const { checks, report, running, error, run } = useDomainDiagnosis(domain)
  const [showPassed, setShowPassed] = useState(false)

  if (error && !running) {
    return (
      <Alert variant="destructive">
        <AlertDescription className="space-y-2">
          <p>
            {error.status === 404
              ? t('diagnose.notFound')
              : t('diagnose.failed', { message: error.message })}
          </p>
          <Button type="button" size="sm" variant="outline" onClick={run}>
            {t('diagnose.tryAgain')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const problems = sortProblems(checks)
  const rest = checks.filter((c) => !NEEDS_FIX.includes(c.state))
  const done = report !== undefined && !running
  return (
    <section className="space-y-3" aria-label={t('diagnose.title', { domain })}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm font-medium" aria-live="polite">
          {running
            ? t('diagnose.running', { count: checks.length })
            : done
              ? t(`diagnose.status.${report.status}`)
              : null}
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={running}
          onClick={run}
        >
          <RefreshCwGlassIcon
            size="1.2em"
            className={running ? 'animate-spin' : undefined}
          />
          {t('diagnose.recheck')}
        </Button>
      </div>
      {report?.go_live ? <GoLiveLink href={report.go_live} /> : null}

      {report?.propagation ? (
        <PropagationLine
          state={report.propagation.state}
          detail={report.propagation.detail}
          seconds={report.propagation.next_check_seconds}
        />
      ) : null}

      {report?.probe_note ? (
        <p className="text-xs text-muted-foreground">{report.probe_note}</p>
      ) : null}

      {checks.length === 0 && running ? (
        <div className="space-y-2" aria-hidden="true">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-3/4" />
        </div>
      ) : null}

      {problems.length > 0 ? (
        <ul className="divide-y divide-border">
          {problems.map((c) => (
            <CheckRow key={c.id} check={c} onApplied={run} />
          ))}
        </ul>
      ) : null}

      {done && problems.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('diagnose.nothingToFix')}
        </p>
      ) : null}

      {rest.length > 0 ? (
        <div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-expanded={showPassed}
            onClick={() => {
              setShowPassed((v) => !v)
            }}
          >
            {showPassed
              ? t('diagnose.hidePassed')
              : t('diagnose.showPassed', { count: rest.length })}
          </Button>
          {showPassed ? (
            <ul className="divide-y divide-border">
              {rest.map((c) => (
                <CheckRow key={c.id} check={c} onApplied={run} />
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}
