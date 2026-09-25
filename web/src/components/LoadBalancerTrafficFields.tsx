import { Field, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import {
  LbTextField,
  LbToggleSection,
  type LbFieldsProps,
} from './LoadBalancerFieldBits'

export function LoadBalancerTrafficFields({
  form,
  errors,
  onChange,
}: LbFieldsProps) {
  return (
    <div className="space-y-3">
      <section className="grid gap-3 rounded-lg border p-4 sm:grid-cols-3">
        <LbTextField
          id="lb-drain-timeout"
          label="Drain on deploy"
          value={form.drainTimeout}
          error={errors.drainTimeout}
          placeholder="15s"
          description="How long open streams survive a cutover."
          onChange={(drainTimeout) => onChange({ drainTimeout })}
        />
        <LbTextField
          id="lb-slow-start"
          label="Slow start"
          value={form.slowStart}
          error={errors.slowStart}
          placeholder="30s"
          description={
            form.algorithm === 'weighted'
              ? 'A new replica ramps up to its weight.'
              : 'Only applies to the weighted algorithm.'
          }
          onChange={(slowStart) => onChange({ slowStart })}
        />
        <LbTextField
          id="lb-request-timeout"
          label="Request timeout"
          value={form.requestTimeout}
          error={errors.requestTimeout}
          placeholder="30s"
          description="Wait for upstream response headers."
          onChange={(requestTimeout) => onChange({ requestTimeout })}
        />
      </section>

      <LbToggleSection
        id="lb-rate"
        title="Rate limit"
        description="Cap requests per client address."
        checked={form.rateEnabled}
        onCheckedChange={(rateEnabled) => onChange({ rateEnabled })}
      >
        <LbTextField
          id="lb-rps"
          label="Requests per second"
          value={form.rps}
          error={errors.rps}
          inputMode="numeric"
          onChange={(rps) => onChange({ rps })}
        />
        <LbTextField
          id="lb-burst"
          label="Burst"
          value={form.burst}
          inputMode="numeric"
          onChange={(burst) => onChange({ burst })}
        />
      </LbToggleSection>

      <LbToggleSection
        id="lb-tls"
        title="TLS to upstreams"
        description="Speak HTTPS to the replicas instead of plain HTTP."
        checked={form.tlsEnabled}
        onCheckedChange={(tlsEnabled) => onChange({ tlsEnabled })}
      >
        <LbTextField
          id="lb-tls-name"
          label="Server name"
          value={form.tlsServerName}
          placeholder="app.internal"
          onChange={(tlsServerName) => onChange({ tlsServerName })}
        />
        <Field orientation="horizontal">
          <Switch
            id="lb-tls-insecure"
            checked={form.tlsInsecure}
            onCheckedChange={(tlsInsecure) => onChange({ tlsInsecure })}
          />
          <FieldLabel htmlFor="lb-tls-insecure">
            Skip certificate verification
          </FieldLabel>
        </Field>
      </LbToggleSection>
    </div>
  )
}
