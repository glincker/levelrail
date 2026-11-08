import { useTranslation } from 'react-i18next'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  LbField,
  LbTextField,
  LbToggleBlock,
  type LbSectionProps,
} from './LbField'

export function ResilienceFields({ form, errors, onChange }: LbSectionProps) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <div className="space-y-6">
      <LbToggleBlock
        id="lb-retries"
        title={t('lb.resilience.retries')}
        info={t('lb.resilience.retriesInfo')}
        checked={form.retriesEnabled}
        onCheckedChange={(retriesEnabled) => onChange({ retriesEnabled })}
      >
        <LbTextField
          id="lb-retry-count"
          label={t('lb.resilience.retries')}
          info={t('lb.resilience.retryCountInfo')}
          value={form.retryCount}
          error={errors.retryCount}
          numeric
          onChange={(retryCount) => onChange({ retryCount })}
        />
        <LbTextField
          id="lb-try-duration"
          label={t('lb.resilience.giveUp')}
          info={t('lb.resilience.giveUpInfo')}
          value={form.tryDuration}
          error={errors.tryDuration}
          placeholder="5s"
          onChange={(tryDuration) => onChange({ tryDuration })}
        />
      </LbToggleBlock>

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <LbTextField
          id="lb-request-timeout"
          label={t('lb.resilience.requestTimeout')}
          info={t('lb.resilience.requestTimeoutInfo')}
          value={form.requestTimeout}
          error={errors.requestTimeout}
          placeholder="30s"
          onChange={(requestTimeout) => onChange({ requestTimeout })}
        />
        <LbTextField
          id="lb-drain-timeout"
          label={t('lb.resilience.drain')}
          info={t('lb.resilience.drainInfo')}
          value={form.drainTimeout}
          error={errors.drainTimeout}
          placeholder="15s"
          onChange={(drainTimeout) => onChange({ drainTimeout })}
        />
      </section>

      <LbToggleBlock
        id="lb-rate"
        title={t('lb.resilience.rate')}
        info={t('lb.resilience.rateInfo')}
        checked={form.rateEnabled}
        onCheckedChange={(rateEnabled) => onChange({ rateEnabled })}
      >
        <LbTextField
          id="lb-rps"
          label={t('lb.resilience.rps')}
          info={t('lb.resilience.rpsInfo')}
          value={form.rps}
          error={errors.rps}
          numeric
          onChange={(rps) => onChange({ rps })}
        />
        <LbTextField
          id="lb-burst"
          label={t('lb.resilience.burst')}
          info={t('lb.resilience.burstInfo')}
          value={form.burst}
          numeric
          onChange={(burst) => onChange({ burst })}
        />
      </LbToggleBlock>

      <LbToggleBlock
        id="lb-tls"
        title={t('lb.resilience.tls')}
        info={t('lb.resilience.tlsInfo')}
        checked={form.tlsEnabled}
        onCheckedChange={(tlsEnabled) => onChange({ tlsEnabled })}
      >
        <LbTextField
          id="lb-tls-name"
          label={t('lb.resilience.serverName')}
          info={t('lb.resilience.serverNameInfo')}
          value={form.tlsServerName}
          placeholder="app.internal"
          onChange={(tlsServerName) => onChange({ tlsServerName })}
        />
        <LbField
          id="lb-tls-insecure"
          label={t('lb.resilience.skipVerify')}
          info={t('lb.resilience.skipVerifyInfo')}
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
              {form.tlsInsecure
                ? t('lb.resilience.notVerified')
                : t('lb.resilience.verified')}
            </Label>
          </div>
        </LbField>
      </LbToggleBlock>
    </div>
  )
}
