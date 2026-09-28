import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  LbField,
  LbTextField,
  LbToggleBlock,
  type LbSectionProps,
} from './LbField'

export function ResilienceFields({ form, errors, onChange }: LbSectionProps) {
  return (
    <div className="space-y-6">
      <LbToggleBlock
        id="lb-retries"
        title="Retries"
        info="Retry a failed request on another replica before giving up. Turn off for streams that cannot be replayed."
        checked={form.retriesEnabled}
        onCheckedChange={(retriesEnabled) => onChange({ retriesEnabled })}
      >
        <LbTextField
          id="lb-retry-count"
          label="Retries"
          info="Extra attempts after the first. 0 to 10."
          value={form.retryCount}
          error={errors.retryCount}
          numeric
          onChange={(retryCount) => onChange({ retryCount })}
        />
        <LbTextField
          id="lb-try-duration"
          label="Give up after"
          info="Total time spent retrying one request."
          value={form.tryDuration}
          error={errors.tryDuration}
          placeholder="5s"
          onChange={(tryDuration) => onChange({ tryDuration })}
        />
      </LbToggleBlock>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <LbTextField
          id="lb-request-timeout"
          label="Request timeout"
          info="How long to wait for a replica to start answering before failing the request."
          value={form.requestTimeout}
          error={errors.requestTimeout}
          placeholder="30s"
          onChange={(requestTimeout) => onChange({ requestTimeout })}
        />
        <LbTextField
          id="lb-drain-timeout"
          label="Drain on deploy"
          info="How long open connections may finish when a replica is replaced during a deploy."
          value={form.drainTimeout}
          error={errors.drainTimeout}
          placeholder="15s"
          onChange={(drainTimeout) => onChange({ drainTimeout })}
        />
      </section>

      <LbToggleBlock
        id="lb-rate"
        title="Rate limit"
        info="Cap requests per client address. Clients above the limit get a 429 response."
        checked={form.rateEnabled}
        onCheckedChange={(rateEnabled) => onChange({ rateEnabled })}
      >
        <LbTextField
          id="lb-rps"
          label="Requests per second"
          info="Sustained rate allowed for each client address."
          value={form.rps}
          error={errors.rps}
          numeric
          onChange={(rps) => onChange({ rps })}
        />
        <LbTextField
          id="lb-burst"
          label="Burst"
          info="Extra requests allowed in a short spike."
          value={form.burst}
          numeric
          onChange={(burst) => onChange({ burst })}
        />
      </LbToggleBlock>

      <LbToggleBlock
        id="lb-tls"
        title="TLS to upstreams"
        info="Speak HTTPS to the replicas instead of plain HTTP."
        checked={form.tlsEnabled}
        onCheckedChange={(tlsEnabled) => onChange({ tlsEnabled })}
      >
        <LbTextField
          id="lb-tls-name"
          label="Server name"
          info="Name the replicas certificate must match."
          value={form.tlsServerName}
          placeholder="app.internal"
          onChange={(tlsServerName) => onChange({ tlsServerName })}
        />
        <LbField
          id="lb-tls-insecure"
          label="Skip verification"
          info="Accept any certificate. Only for testing: it removes protection against impersonation."
        >
          <div className="flex items-center gap-2">
            <Switch
              id="lb-tls-insecure"
              checked={form.tlsInsecure}
              onCheckedChange={(tlsInsecure) => onChange({ tlsInsecure })}
            />
            <Label
              htmlFor="lb-tls-insecure"
              className="text-xs text-muted-foreground"
            >
              {form.tlsInsecure ? 'Certificates not verified' : 'Verified'}
            </Label>
          </div>
        </LbField>
      </LbToggleBlock>
    </div>
  )
}
