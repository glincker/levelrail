import { ChevronDownGlassIcon, ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { StatusPill, type Tone } from './kit'
import {
  useAllowSecretFingerprint,
  useDeploymentSecretScan,
} from '../queries/secretScan'
import type {
  SecretFinding,
  SecretScanAction,
  SecretScanRun,
} from '../types/secretScan'

const ACTION_TONE: Record<SecretScanAction, Tone> = {
  skipped: 'neutral',
  allow: 'success',
  warn: 'warning',
  block: 'danger',
}

// Short enough to scan by eye; the full fingerprint stays in the title.
const FINGERPRINT_CHARS = 10

function FindingRow({
  finding,
  appName,
  reason,
}: {
  finding: SecretFinding
  appName: string
  reason: string
}) {
  const { t } = useTranslation('secretScan')
  const allow = useAllowSecretFingerprint(appName)
  const location =
    finding.source === 'env'
      ? t('section.locationEnv', { name: finding.var })
      : `${finding.path ?? ''}:${finding.line ?? 0}`

  function onAllow() {
    allow.mutate(
      { fingerprint: finding.fingerprint, reason: reason.trim() },
      {
        onSuccess: () =>
          toast.add({ title: t('section.toastAllowed'), type: 'success' }),
        onError: (error) =>
          toast.add({
            title: t('section.toastAllowFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  const rule = t(`rule.${finding.rule_id}`, { defaultValue: finding.rule_id })
  return (
    <li className="flex items-center gap-2 px-3 py-1 text-xs">
      <span className="w-40 shrink-0 truncate font-medium" title={rule}>
        {rule}
      </span>
      <span
        className="min-w-0 flex-1 truncate font-mono text-muted-foreground"
        title={location}
      >
        {location}
      </span>
      <span
        className="hidden shrink-0 font-mono text-muted-foreground sm:inline"
        title={finding.fingerprint}
      >
        {finding.fingerprint.slice(0, FINGERPRINT_CHARS)}
      </span>
      {finding.allowed ? (
        <StatusPill tone="neutral" label={t('section.allowed')} size="sm" />
      ) : (
        <Button
          size="sm"
          variant="outline"
          onClick={onAllow}
          disabled={reason.trim() === '' || allow.isPending}
        >
          {t('section.allow')}
        </Button>
      )}
    </li>
  )
}

function Header({
  open,
  onToggle,
  controls,
  summary,
  run,
}: {
  open: boolean
  onToggle: () => void
  controls: string
  summary: string
  run?: SecretScanRun
}) {
  const { t } = useTranslation('secretScan')
  const Caret = open ? ChevronDownGlassIcon : ChevronRightGlassIcon
  return (
    <h3 className="m-0">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={controls}
        onClick={onToggle}
        className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        <Caret
          className="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="text-sm font-semibold text-foreground">
          {t('section.title')}
        </span>
        <span className="min-w-0 truncate text-xs text-muted-foreground">
          {summary}
        </span>
        {run ? (
          <span className="ml-auto flex shrink-0 items-center gap-2">
            <StatusPill
              tone={ACTION_TONE[run.action]}
              label={t(`action.${run.action}`)}
              size="sm"
            />
          </span>
        ) : null}
      </button>
    </h3>
  )
}

function RunBlock({ appName, run }: { appName: string; run: SecretScanRun }) {
  const { t } = useTranslation('secretScan')
  const [reason, setReason] = useState('')
  // A blocking scan stopped the deploy, so its findings are what the reader needs first.
  const [open, setOpen] = useState(run.action === 'block')
  const hasActive = run.findings.some((f) => !f.allowed)
  const activeCount = run.findings.filter((f) => !f.allowed).length
  const bodyId = `secret-scan-${run.deployment_id}`
  const summary =
    run.findings.length === 0
      ? t('section.clean')
      : t('section.summary', { count: activeCount })
  return (
    <section className="rounded-lg border border-border bg-card">
      <Header
        open={open}
        onToggle={() => {
          setOpen((v) => !v)
        }}
        controls={bodyId}
        summary={summary}
        run={run}
      />
      {open ? (
        <div id={bodyId} className="space-y-2 border-t border-border/60 py-2">
          <p className="px-3 text-xs text-muted-foreground">
            {t('section.files', { count: run.files_scanned })}
            {run.reason ? ` ${run.reason}` : ''}
            {run.truncated ? ` ${t('section.partial')}` : ''}
          </p>
          {run.findings.length > 0 ? (
            <>
              {hasActive ? (
                <div className="px-3">
                  <Input
                    aria-label={t('section.allowReason')}
                    value={reason}
                    onChange={(e) => {
                      setReason(e.target.value)
                    }}
                    placeholder={t('section.allowReason')}
                    className="h-8 text-xs"
                  />
                </div>
              ) : null}
              <ul
                className="max-h-64 divide-y divide-border/60 overflow-y-auto"
                aria-label={t('section.findings')}
              >
                {run.findings.map((f) => (
                  <FindingRow
                    key={`${f.fingerprint}:${f.path ?? f.var ?? ''}:${f.line ?? 0}`}
                    finding={f}
                    appName={appName}
                    reason={reason}
                  />
                ))}
              </ul>
            </>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function StaticBlock({ summary }: { summary: string }) {
  const { t } = useTranslation('secretScan')
  return (
    <section className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2">
      <h3 className="m-0 text-sm font-semibold text-foreground">
        {t('section.title')}
      </h3>
      <span className="text-xs text-muted-foreground">{summary}</span>
    </section>
  )
}

// SecretScanSection is a one-line summary of the deploy-time secret scan; findings expand below it.
export function SecretScanSection({
  appName,
  deploymentId,
}: {
  appName: string
  deploymentId: string
}) {
  const { t } = useTranslation('secretScan')
  const scan = useDeploymentSecretScan(appName, deploymentId)
  if (scan.isPending) {
    return (
      <div aria-busy="true" aria-label={t('section.loading')}>
        <Skeleton className="h-9 w-full rounded-lg" />
      </div>
    )
  }
  if (scan.isError) return <StaticBlock summary={t('section.empty')} />
  return <RunBlock appName={appName} run={scan.data} />
}
