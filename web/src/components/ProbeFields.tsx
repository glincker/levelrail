import {
  Controller,
  useWatch,
  type Control,
  type FormState,
  type UseFormRegister,
} from 'react-hook-form'
import type { ServiceProbe } from '../types/appDetail'
import type { ProbeFormValues } from '../lib/healthProbeForm'
import { describeProbe } from '../lib/healthProbeForm'
import { formatDurationNs } from '../lib/format'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

export interface HealthFormValues {
  readiness: ProbeFormValues
  liveness: ProbeFormValues
}

type Prefix = 'readiness' | 'liveness'
type BoolField =
  'enabled' | 'useExec' | 'https' | 'tlsSkipVerify' | 'followRedirects'

interface ProbeFieldsProps {
  title: string
  fieldPrefix: Prefix
  control: Control<HealthFormValues>
  register: UseFormRegister<HealthFormValues>
  formState: FormState<HealthFormValues>
  currentProbe?: ServiceProbe | null
}

function ToggleField({
  control,
  fieldPrefix,
  name,
  label,
  description,
  disabled,
}: {
  control: Control<HealthFormValues>
  fieldPrefix: Prefix
  name: BoolField
  label: string
  description?: string
  disabled?: boolean
}) {
  const id = `${fieldPrefix}-${name}`
  return (
    <Field orientation="horizontal">
      <Controller
        control={control}
        name={`${fieldPrefix}.${name}`}
        render={({ field }) => (
          <Switch
            id={id}
            checked={field.value}
            onCheckedChange={field.onChange}
            disabled={disabled}
          />
        )}
      />
      <div>
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        {description ? (
          <FieldDescription>{description}</FieldDescription>
        ) : null}
      </div>
    </Field>
  )
}

export function ProbeFields({
  title,
  fieldPrefix,
  control,
  register,
  formState,
  currentProbe,
}: ProbeFieldsProps) {
  const errors = formState.errors[fieldPrefix]
  const value = useWatch({ control, name: fieldPrefix })
  const currentSummary = currentProbe
    ? `Currently: ${describeProbe(currentProbe)}, every ${formatDurationNs(currentProbe.interval)}`
    : 'Currently: not configured'

  return (
    <div className="rounded-md border border-border p-3">
      <ToggleField
        control={control}
        fieldPrefix={fieldPrefix}
        name="enabled"
        label={title}
      />
      <FieldDescription className="mt-1">{currentSummary}</FieldDescription>

      {value.enabled ? (
        <FieldGroup className="mt-3 gap-2">
          <ToggleField
            control={control}
            fieldPrefix={fieldPrefix}
            name="useExec"
            label="Run a command instead of HTTP"
            description="Exit code 0 inside the container means healthy."
          />
          {value.useExec ? (
            <Field>
              <FieldLabel htmlFor={`${fieldPrefix}-exec`}>Command</FieldLabel>
              <Input
                id={`${fieldPrefix}-exec`}
                {...register(`${fieldPrefix}.execCommand`)}
                className="font-mono"
                placeholder="pg_isready -U app"
              />
              <FieldDescription>
                Runs through /bin/sh -c inside the container.
              </FieldDescription>
              <FieldError errors={[errors?.execCommand]} />
            </Field>
          ) : (
            <HttpFields
              fieldPrefix={fieldPrefix}
              control={control}
              register={register}
              formState={formState}
              https={value.https}
            />
          )}
          <TimingFields
            fieldPrefix={fieldPrefix}
            register={register}
            formState={formState}
          />
        </FieldGroup>
      ) : (
        <p className="mt-3 text-sm text-muted-foreground">
          Probe is not configured.
        </p>
      )}
    </div>
  )
}

function HttpFields({
  fieldPrefix,
  control,
  register,
  formState,
  https,
}: Omit<ProbeFieldsProps, 'title' | 'currentProbe'> & { https: boolean }) {
  const errors = formState.errors[fieldPrefix]
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-path`}>Path</FieldLabel>
        <Input
          id={`${fieldPrefix}-path`}
          {...register(`${fieldPrefix}.path`)}
          className="font-mono"
          placeholder="/healthz"
        />
        <FieldError errors={[errors?.path]} />
      </Field>
      <ToggleField
        control={control}
        fieldPrefix={fieldPrefix}
        name="https"
        label="HTTPS"
      />
      <ToggleField
        control={control}
        fieldPrefix={fieldPrefix}
        name="tlsSkipVerify"
        label="Skip TLS verification"
        description="For a self-signed certificate inside the container."
        disabled={!https}
      />
      <FieldError errors={[errors?.tlsSkipVerify]} />
      <ToggleField
        control={control}
        fieldPrefix={fieldPrefix}
        name="followRedirects"
        label="Follow redirects"
        description="Off judges the 3xx response itself against the expected status."
      />
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-expected-status`}>
          Expected status
        </FieldLabel>
        <Input
          id={`${fieldPrefix}-expected-status`}
          {...register(`${fieldPrefix}.expectedStatus`)}
          className="font-mono"
          placeholder="200-299"
        />
        <FieldDescription>
          Codes or ranges, e.g. 200-399 or 200,204.
        </FieldDescription>
        <FieldError errors={[errors?.expectedStatus]} />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-host`}>Host header</FieldLabel>
        <Input
          id={`${fieldPrefix}-host`}
          {...register(`${fieldPrefix}.host`)}
          className="font-mono"
          placeholder="app.example.com"
        />
        <FieldError errors={[errors?.host]} />
      </Field>
    </>
  )
}

function TimingFields({
  fieldPrefix,
  register,
  formState,
}: Pick<ProbeFieldsProps, 'fieldPrefix' | 'register' | 'formState'>) {
  const errors = formState.errors[fieldPrefix]
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-interval`}>
          Interval (seconds)
        </FieldLabel>
        <Input
          id={`${fieldPrefix}-interval`}
          {...register(`${fieldPrefix}.intervalSeconds`)}
          inputMode="decimal"
          placeholder="5"
        />
        <FieldError errors={[errors?.intervalSeconds]} />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-timeout`}>
          Timeout (seconds)
        </FieldLabel>
        <Input
          id={`${fieldPrefix}-timeout`}
          {...register(`${fieldPrefix}.timeoutSeconds`)}
          inputMode="decimal"
          placeholder="2"
        />
        <FieldError errors={[errors?.timeoutSeconds]} />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${fieldPrefix}-failures`}>
          Failure threshold
        </FieldLabel>
        <Input
          id={`${fieldPrefix}-failures`}
          {...register(`${fieldPrefix}.failures`)}
          inputMode="numeric"
          placeholder="3"
        />
        <FieldError errors={[errors?.failures]} />
      </Field>
    </>
  )
}
