import { HeartbeatIcon } from '@phosphor-icons/react/dist/ssr'
import { detectionTimes } from './detection'
import { LbTextField, LbToggleBlock, type LbSectionProps } from './LbField'

export function HealthDetectionPreview({ form }: Pick<LbSectionProps, 'form'>) {
  const d = detectionTimes(form)
  if (!d) return null
  return (
    <p
      data-testid="detection-preview"
      className="flex items-center gap-2 rounded-lg bg-muted/60 px-3 py-2 text-sm"
    >
      <HeartbeatIcon className="size-4 shrink-0 text-tone-info" aria-hidden />
      <span>{d.text}</span>
    </p>
  )
}

export function HealthFields({ form, errors, onChange }: LbSectionProps) {
  return (
    <div className="space-y-6">
      <LbToggleBlock
        id="lb-active"
        title="Active checks"
        info="Probe every replica on an interval and stop sending traffic to one that fails."
        checked={form.healthEnabled}
        onCheckedChange={(healthEnabled) => onChange({ healthEnabled })}
        footer={<HealthDetectionPreview form={form} />}
      >
        <LbTextField
          id="lb-health-path"
          label="Path"
          info="The URL probed on each replica. It should return quickly and only depend on the app itself."
          value={form.healthPath}
          error={errors.healthPath}
          placeholder="/healthz"
          onChange={(healthPath) => onChange({ healthPath })}
        />
        <LbTextField
          id="lb-health-status"
          label="Expected status"
          info="The HTTP status that counts as a pass. Blank accepts any 2xx or 3xx."
          value={form.healthStatus}
          error={errors.healthStatus}
          placeholder="2xx or 3xx"
          numeric
          onChange={(healthStatus) => onChange({ healthStatus })}
        />
        <LbTextField
          id="lb-health-interval"
          label="Interval"
          info="How often each replica is probed."
          value={form.healthInterval}
          error={errors.healthInterval}
          placeholder="10s"
          onChange={(healthInterval) => onChange({ healthInterval })}
        />
        <LbTextField
          id="lb-health-timeout"
          label="Timeout"
          info="How long to wait for a reply. Must be shorter than the interval."
          value={form.healthTimeout}
          error={errors.healthTimeout}
          placeholder="5s"
          onChange={(healthTimeout) => onChange({ healthTimeout })}
        />
        <LbTextField
          id="lb-health-fails"
          label="Failures to mark down"
          info="Consecutive failed probes before a replica leaves the pool."
          value={form.healthFails}
          placeholder="1"
          numeric
          onChange={(healthFails) => onChange({ healthFails })}
        />
        <LbTextField
          id="lb-health-passes"
          label="Passes to recover"
          info="Consecutive passing probes before a replica rejoins the pool."
          value={form.healthPasses}
          placeholder="1"
          numeric
          onChange={(healthPasses) => onChange({ healthPasses })}
        />
      </LbToggleBlock>

      <LbToggleBlock
        id="lb-passive"
        title="Passive checks"
        info="Watch real requests. A replica that keeps failing them is skipped for a while, faster than waiting for the next probe."
        checked={form.passiveEnabled}
        onCheckedChange={(passiveEnabled) => onChange({ passiveEnabled })}
      >
        <LbTextField
          id="lb-max-fails"
          label="Failures before skipping"
          info="Failed requests within the window that trigger a skip."
          value={form.maxFails}
          placeholder="1"
          numeric
          onChange={(maxFails) => onChange({ maxFails })}
        />
        <LbTextField
          id="lb-fail-duration"
          label="Skip for"
          info="How long a failing replica is skipped, and the window failures are counted in."
          value={form.failDuration}
          error={errors.failDuration}
          placeholder="30s"
          onChange={(failDuration) => onChange({ failDuration })}
        />
      </LbToggleBlock>
    </div>
  )
}
