import {
  LbTextField,
  LbToggleSection,
  type LbFieldsProps,
} from './LoadBalancerFieldBits'

export function LoadBalancerHealthFields({
  form,
  errors,
  onChange,
}: LbFieldsProps) {
  return (
    <div className="space-y-3">
      <LbToggleSection
        id="lb-active"
        title="Active health checks"
        description="Probe every replica on an interval and stop sending traffic to one that fails."
        checked={form.healthEnabled}
        onCheckedChange={(healthEnabled) => onChange({ healthEnabled })}
      >
        <LbTextField
          id="lb-health-path"
          label="Path"
          value={form.healthPath}
          error={errors.healthPath}
          placeholder="/healthz"
          onChange={(healthPath) => onChange({ healthPath })}
        />
        <LbTextField
          id="lb-health-status"
          label="Expected status"
          value={form.healthStatus}
          error={errors.healthStatus}
          placeholder="any 2xx or 3xx"
          inputMode="numeric"
          onChange={(healthStatus) => onChange({ healthStatus })}
        />
        <LbTextField
          id="lb-health-interval"
          label="Interval"
          value={form.healthInterval}
          error={errors.healthInterval}
          placeholder="10s"
          onChange={(healthInterval) => onChange({ healthInterval })}
        />
        <LbTextField
          id="lb-health-timeout"
          label="Timeout"
          value={form.healthTimeout}
          error={errors.healthTimeout}
          placeholder="5s"
          onChange={(healthTimeout) => onChange({ healthTimeout })}
        />
        <LbTextField
          id="lb-health-passes"
          label="Passes to recover"
          value={form.healthPasses}
          placeholder="1"
          inputMode="numeric"
          onChange={(healthPasses) => onChange({ healthPasses })}
        />
        <LbTextField
          id="lb-health-fails"
          label="Failures to mark down"
          value={form.healthFails}
          placeholder="1"
          inputMode="numeric"
          onChange={(healthFails) => onChange({ healthFails })}
        />
      </LbToggleSection>

      <LbToggleSection
        id="lb-passive"
        title="Passive health checks"
        description="Skip a replica for a while after it fails real requests."
        checked={form.passiveEnabled}
        onCheckedChange={(passiveEnabled) => onChange({ passiveEnabled })}
      >
        <LbTextField
          id="lb-max-fails"
          label="Failures before skipping"
          value={form.maxFails}
          placeholder="1"
          inputMode="numeric"
          onChange={(maxFails) => onChange({ maxFails })}
        />
        <LbTextField
          id="lb-fail-duration"
          label="Skip for"
          value={form.failDuration}
          error={errors.failDuration}
          placeholder="30s"
          onChange={(failDuration) => onChange({ failDuration })}
        />
      </LbToggleSection>

      <LbToggleSection
        id="lb-retries"
        title="Retries"
        description="Retry a failed request on another replica before giving up."
        checked={form.retriesEnabled}
        onCheckedChange={(retriesEnabled) => onChange({ retriesEnabled })}
      >
        <LbTextField
          id="lb-retry-count"
          label="Retries"
          value={form.retryCount}
          error={errors.retryCount}
          inputMode="numeric"
          onChange={(retryCount) => onChange({ retryCount })}
        />
        <LbTextField
          id="lb-try-duration"
          label="Give up after"
          value={form.tryDuration}
          error={errors.tryDuration}
          placeholder="5s"
          onChange={(tryDuration) => onChange({ tryDuration })}
        />
      </LbToggleSection>
    </div>
  )
}
