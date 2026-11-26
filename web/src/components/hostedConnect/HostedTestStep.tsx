import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { probeReached, tlsOutcome, type HostedForm } from '@/lib/hostedConnect'
import type { ExternalProbeResult } from '../../types/externalDatabase'
import { ExternalHealthBadge } from '../ExternalHealthBadge'

const TLS_KEY = {
  verified: 'external.hosted.tlsVerified',
  unverified: 'external.hosted.tlsNotVerified',
  notChecked: 'external.hosted.tlsNotChecked',
  off: 'external.hosted.tlsOff',
} as const

interface Props {
  form: HostedForm
  probe: ExternalProbeResult | null
  testing: boolean
  failed: boolean
  error: string | null
  saveAnyway: boolean
  onSaveAnyway: (v: boolean) => void
  onTest: () => void
}

// Step 3: probe before saving. The save-anyway box only exists after a
// failed probe, so a green path never offers to skip the check.
export function HostedTestStep({
  form,
  probe,
  testing,
  failed,
  error,
  saveAnyway,
  onSaveAnyway,
  onTest,
}: Readonly<Props>) {
  const { t } = useTranslation('databases')
  const reached = probe ? probeReached(probe) : false
  return (
    <div className="space-y-3">
      <p className="text-sm">
        {t('external.hosted.summary', {
          engine: form.engine,
          host: form.host,
          port: form.port,
        })}
      </p>
      <p className="text-xs text-muted-foreground">
        {t('external.hosted.readOnlyNote')}
      </p>

      <div className="space-y-2 rounded-lg border border-border p-3">
        <div className="flex items-center justify-between gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={testing}
            onClick={onTest}
          >
            {testing
              ? t('external.hosted.testing')
              : probe
                ? t('external.hosted.retest')
                : t('external.hosted.test')}
          </Button>
          {probe ? <ExternalHealthBadge status={probe.status} /> : null}
        </div>

        {probe ? null : (
          <p className="text-xs text-muted-foreground">
            {t('external.hosted.testIdle')}
          </p>
        )}

        {probe ? (
          <div className="space-y-1 text-sm" aria-live="polite">
            <p className="flex items-start gap-1.5">
              {reached ? (
                <CircleCheckGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-success"
                  aria-hidden="true"
                />
              ) : (
                <TriangleAlertGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-tone-warning"
                  aria-hidden="true"
                />
              )}
              {reached
                ? t('external.hosted.reachable')
                : t(`external.health.next.${probe.status}`)}
            </p>
            {probe.reason ? (
              <p className="font-mono text-xs break-words text-muted-foreground">
                {probe.reason}
              </p>
            ) : null}
            {reached ? (
              <ul className="space-y-0.5 text-xs text-muted-foreground">
                <li>{t(TLS_KEY[tlsOutcome(form.tls, probe)])}</li>
                {probe.version ? (
                  <li>
                    {t('external.hosted.server', { version: probe.version })}
                  </li>
                ) : null}
                <li>
                  {t('external.hosted.latency', { ms: probe.latency_ms })}
                </li>
              </ul>
            ) : null}
          </div>
        ) : null}
      </div>

      {error ? (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      {failed ? (
        <div className="space-y-1.5 rounded-lg border border-border bg-tone-warning-soft p-3">
          <label className="flex items-center gap-2 text-sm font-medium">
            <Checkbox
              checked={saveAnyway}
              onCheckedChange={(v) => {
                onSaveAnyway(v === true)
              }}
            />
            {t('external.hosted.saveAnyway')}
          </label>
          <p className="text-xs text-muted-foreground">
            {t('external.hosted.saveAnywayWarn')}
          </p>
        </div>
      ) : null}
    </div>
  )
}
