import { CardSkeleton } from '@/components/ui/card-skeleton'
import { useState, type ReactNode } from 'react'
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

  return (
    <li className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border p-2 text-sm">
      <div className="min-w-0 space-y-0.5">
        <p className="font-medium">
          {t(`rule.${finding.rule_id}`, { defaultValue: finding.rule_id })}
        </p>
        <p className="break-all font-mono text-xs text-muted-foreground">
          {location}
        </p>
        <p className="font-mono text-xs text-muted-foreground">
          {finding.fingerprint}
        </p>
      </div>
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

function RunBody({ appName, run }: { appName: string; run: SecretScanRun }) {
  const { t } = useTranslation('secretScan')
  const [reason, setReason] = useState('')
  const hasActive = run.findings.some((f) => !f.allowed)
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill
          tone={ACTION_TONE[run.action]}
          label={t(`action.${run.action}`)}
          size="sm"
        />
        <span className="text-xs text-muted-foreground">
          {t('section.files', { count: run.files_scanned })}
        </span>
      </div>
      {run.reason ? (
        <p className="text-sm text-muted-foreground">{run.reason}</p>
      ) : null}
      {run.truncated ? (
        <p className="text-sm text-muted-foreground">{t('section.partial')}</p>
      ) : null}
      {run.findings.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('section.clean')}</p>
      ) : (
        <>
          {hasActive ? (
            <Input
              aria-label={t('section.allowReason')}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={t('section.allowReason')}
            />
          ) : null}
          <ul className="space-y-2" aria-label={t('section.findings')}>
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
      )}
    </div>
  )
}

// SecretScanSection shows what the deploy-time secret scan found for one deploy.
export function SecretScanSection({
  appName,
  deploymentId,
}: {
  appName: string
  deploymentId: string
}) {
  const { t } = useTranslation('secretScan')
  const scan = useDeploymentSecretScan(appName, deploymentId)
  let body: ReactNode
  if (scan.isPending) {
    body = <CardSkeleton titleWidth="w-40" rows={2} />
  } else if (scan.isError) {
    body = <p className="text-sm text-muted-foreground">{t('section.empty')}</p>
  } else {
    body = <RunBody appName={appName} run={scan.data} />
  }
  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t('section.title')}
      </h3>
      {body}
    </section>
  )
}
