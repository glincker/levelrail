import { useTranslation } from 'react-i18next'
import { ActivityGlassIcon } from '@thesvg/ui-react'
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
      <ActivityGlassIcon
        className="size-5 shrink-0 text-tone-info"
        aria-hidden
      />
      <span>{d.text}</span>
    </p>
  )
}

export function HealthFields({ form, errors, onChange }: LbSectionProps) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <div className="space-y-6">
      <LbToggleBlock
        id="lb-active"
        title={t('lb.health.active')}
        info={t('lb.health.activeInfo')}
        checked={form.healthEnabled}
        onCheckedChange={(healthEnabled) => onChange({ healthEnabled })}
        footer={<HealthDetectionPreview form={form} />}
      >
        <LbTextField
          id="lb-health-path"
          label={t('lb.health.path')}
          info={t('lb.health.pathInfo')}
          value={form.healthPath}
          error={errors.healthPath}
          placeholder="/healthz"
          onChange={(healthPath) => onChange({ healthPath })}
        />
        <LbTextField
          id="lb-health-status"
          label={t('lb.health.status')}
          info={t('lb.health.statusInfo')}
          value={form.healthStatus}
          error={errors.healthStatus}
          placeholder={t('lb.health.statusPlaceholder')}
          numeric
          onChange={(healthStatus) => onChange({ healthStatus })}
        />
        <LbTextField
          id="lb-health-interval"
          label={t('lb.health.interval')}
          info={t('lb.health.intervalInfo')}
          value={form.healthInterval}
          error={errors.healthInterval}
          placeholder="10s"
          onChange={(healthInterval) => onChange({ healthInterval })}
        />
        <LbTextField
          id="lb-health-timeout"
          label={t('lb.health.timeout')}
          info={t('lb.health.timeoutInfo')}
          value={form.healthTimeout}
          error={errors.healthTimeout}
          placeholder="5s"
          onChange={(healthTimeout) => onChange({ healthTimeout })}
        />
        <LbTextField
          id="lb-health-fails"
          label={t('lb.health.fails')}
          info={t('lb.health.failsInfo')}
          value={form.healthFails}
          placeholder="1"
          numeric
          onChange={(healthFails) => onChange({ healthFails })}
        />
        <LbTextField
          id="lb-health-passes"
          label={t('lb.health.passes')}
          info={t('lb.health.passesInfo')}
          value={form.healthPasses}
          placeholder="1"
          numeric
          onChange={(healthPasses) => onChange({ healthPasses })}
        />
      </LbToggleBlock>

      <LbToggleBlock
        id="lb-passive"
        title={t('lb.health.passive')}
        info={t('lb.health.passiveInfo')}
        checked={form.passiveEnabled}
        onCheckedChange={(passiveEnabled) => onChange({ passiveEnabled })}
      >
        <LbTextField
          id="lb-max-fails"
          label={t('lb.health.maxFails')}
          info={t('lb.health.maxFailsInfo')}
          value={form.maxFails}
          placeholder="1"
          numeric
          onChange={(maxFails) => onChange({ maxFails })}
        />
        <LbTextField
          id="lb-fail-duration"
          label={t('lb.health.skipFor')}
          info={t('lb.health.skipForInfo')}
          value={form.failDuration}
          error={errors.failDuration}
          placeholder="30s"
          onChange={(failDuration) => onChange({ failDuration })}
        />
      </LbToggleBlock>
    </div>
  )
}
