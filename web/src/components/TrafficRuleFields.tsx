import { Controller, type Control, type FieldErrors } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { DurationInput } from '@/components/ui/duration-input'
import { domainsQueryOptions } from '../queries/domains'
import { PARKED_CERT_KIND } from '@/lib/alertTrafficKinds'

export interface TrafficRuleFormShape {
  trafficDomain: string
  forDuration: string
  threshold: number
  restartCountThreshold: number
}

const ALL_DOMAINS = '__all__'

/** Fields for domain_traffic_drop and parked_domain_cert_expiry. */
export function TrafficRuleFields({
  idPrefix,
  kind,
  appName,
  control,
  errors,
  register,
}: {
  idPrefix: string
  kind: string
  appName: string
  control: Control<TrafficRuleFormShape>
  errors: FieldErrors<TrafficRuleFormShape>
  register: (
    name: 'threshold' | 'restartCountThreshold',
  ) => Record<string, unknown>
}) {
  const { t } = useTranslation('alertSetup')
  const domainsQuery = useQuery({
    ...domainsQueryOptions(),
    enabled: kind !== PARKED_CERT_KIND,
  })
  const domains = (domainsQuery.data ?? []).filter(
    (d) => d.service_name === appName,
  )

  return (
    <>
      <p className="text-sm text-muted-foreground">
        {kind === PARKED_CERT_KIND
          ? t('trafficKinds.kind.parked_domain_cert_expiry.help')
          : t('trafficKinds.kind.domain_traffic_drop.help')}
      </p>
      {kind === PARKED_CERT_KIND ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-days-left`}>
            {t('trafficKinds.daysLabel')}
          </FieldLabel>
          <Input
            id={`${idPrefix}-days-left`}
            type="number"
            step="1"
            min="1"
            {...register('threshold')}
          />
          <FieldDescription>{t('trafficKinds.daysHelp')}</FieldDescription>
          <FieldError errors={[errors.threshold]} />
        </Field>
      ) : (
        <>
          <Field>
            <FieldLabel htmlFor={`${idPrefix}-traffic-domain`}>
              {t('trafficKinds.scopeLabel')}
            </FieldLabel>
            <Controller
              control={control}
              name="trafficDomain"
              render={({ field }) => (
                <Select
                  value={field.value || ALL_DOMAINS}
                  onValueChange={(next) =>
                    field.onChange(next === ALL_DOMAINS ? '' : next)
                  }
                >
                  <SelectTrigger
                    id={`${idPrefix}-traffic-domain`}
                    className="w-full"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={ALL_DOMAINS}>
                      {t('trafficKinds.scopeAll')}
                    </SelectItem>
                    {domains.map((d) => (
                      <SelectItem key={d.domain} value={d.domain}>
                        {d.domain}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            <FieldDescription>
              {t('trafficKinds.scopeApp', { app: appName })}.{' '}
              {t('trafficKinds.scopeHelp')}
            </FieldDescription>
          </Field>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field>
              <FieldLabel htmlFor={`${idPrefix}-traffic-window`}>
                {t('trafficKinds.windowLabel')}
              </FieldLabel>
              <Controller
                control={control}
                name="forDuration"
                render={({ field }) => (
                  <DurationInput
                    id={`${idPrefix}-traffic-window`}
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                  />
                )}
              />
              <FieldError errors={[errors.forDuration]} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${idPrefix}-traffic-percent`}>
                {t('trafficKinds.percentLabel')}
              </FieldLabel>
              <Input
                id={`${idPrefix}-traffic-percent`}
                type="number"
                step="any"
                min="0"
                max="100"
                {...register('threshold')}
              />
              <FieldError errors={[errors.threshold]} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${idPrefix}-traffic-min`}>
                {t('trafficKinds.minRequestsLabel')}
              </FieldLabel>
              <Input
                id={`${idPrefix}-traffic-min`}
                type="number"
                step="1"
                min="1"
                {...register('restartCountThreshold')}
              />
              <FieldError errors={[errors.restartCountThreshold]} />
            </Field>
          </div>
          <FieldDescription>
            {t('trafficKinds.windowHelp')} {t('trafficKinds.percentHelp')}{' '}
            {t('trafficKinds.minRequestsHelp')}
          </FieldDescription>
        </>
      )}
    </>
  )
}
