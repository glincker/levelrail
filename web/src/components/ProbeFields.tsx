import { useState } from 'react'
import {
  Controller,
  useWatch,
  type Control,
  type FormState,
  type UseFormRegister,
  type UseFormSetValue,
} from 'react-hook-form'
import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import type { ReactNode } from 'react'
import type { ServiceProbe } from '../types/appDetail'
import type { ProbeFormValues } from '../lib/healthProbeForm'
import { describeProbe } from '../lib/healthProbeForm'
import {
  HEALTH_CHECK_PATH_PRESETS,
  probeTimingDefaults,
} from '../lib/healthCheckDefaults'
import { formatDurationNs } from '../lib/format'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Button } from '@/components/ui/button'
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
  setValue: UseFormSetValue<HealthFormValues>
  formState: FormState<HealthFormValues>
  currentProbe?: ServiceProbe | null
  headerAction?: ReactNode
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

function isCustomPath(path: string): boolean {
  return path !== '' && !HEALTH_CHECK_PATH_PRESETS.some((p) => p.path === path)
}

export function ProbeFields({
  title,
  fieldPrefix,
  control,
  register,
  setValue,
  formState,
  currentProbe,
  headerAction,
}: ProbeFieldsProps) {
  const errors = formState.errors[fieldPrefix]
  const value = useWatch({ control, name: fieldPrefix })
  // Caller passes key={app.name} so this remounts (and re-derives this
  // local UI-only choice) when the editor is reused for a different app.
  const [customPath, setCustomPath] = useState(() =>
    isCustomPath(currentProbe?.path ?? ''),
  )

  const currentSummary = currentProbe
    ? `Currently: ${describeProbe(currentProbe)}, every ${formatDurationNs(currentProbe.interval)}`
    : 'Currently: not configured'

  function applyPreset(path: string) {
    setCustomPath(false)
    const defaults = probeTimingDefaults(fieldPrefix)
    setValue(`${fieldPrefix}.useExec`, false, { shouldDirty: true })
    setValue(`${fieldPrefix}.path`, path, {
      shouldDirty: true,
      shouldValidate: true,
    })
    setValue(`${fieldPrefix}.intervalSeconds`, defaults.intervalSeconds, {
      shouldDirty: true,
    })
    setValue(`${fieldPrefix}.timeoutSeconds`, defaults.timeoutSeconds, {
      shouldDirty: true,
    })
    setValue(`${fieldPrefix}.failures`, defaults.failures, {
      shouldDirty: true,
    })
  }

  function useCustomPath() {
    setCustomPath(true)
    setValue(`${fieldPrefix}.useExec`, false, { shouldDirty: true })
    const defaults = probeTimingDefaults(fieldPrefix)
    if (value.intervalSeconds === '') {
      setValue(`${fieldPrefix}.intervalSeconds`, defaults.intervalSeconds, {
        shouldDirty: true,
      })
    }
    if (value.timeoutSeconds === '' && defaults.timeoutSeconds !== '') {
      setValue(`${fieldPrefix}.timeoutSeconds`, defaults.timeoutSeconds, {
        shouldDirty: true,
      })
    }
    if (value.failures === '' && defaults.failures !== '') {
      setValue(`${fieldPrefix}.failures`, defaults.failures, {
        shouldDirty: true,
      })
    }
  }

  function disableProbe() {
    setValue(`${fieldPrefix}.enabled`, false, { shouldDirty: true })
  }

  const activePresetId = !customPath
    ? HEALTH_CHECK_PATH_PRESETS.find((p) => p.path === value.path)?.id
    : undefined

  return (
    <div className="rounded-md border border-border p-3">
      <div className="flex items-center justify-between gap-2">
        <ToggleField
          control={control}
          fieldPrefix={fieldPrefix}
          name="enabled"
          label={title}
        />
        {headerAction}
      </div>
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
            <>
              <Field>
                <FieldLabel>Health path</FieldLabel>
                <div
                  className="flex flex-wrap gap-1.5"
                  role="group"
                  aria-label={`${title} path preset`}
                >
                  {HEALTH_CHECK_PATH_PRESETS.map((preset) => (
                    <Button
                      key={preset.id}
                      type="button"
                      size="xs"
                      variant={
                        activePresetId === preset.id ? 'secondary' : 'outline'
                      }
                      onClick={() => applyPreset(preset.path)}
                    >
                      {preset.path}
                    </Button>
                  ))}
                  <Button
                    type="button"
                    size="xs"
                    variant={customPath ? 'secondary' : 'outline'}
                    onClick={useCustomPath}
                  >
                    Custom path
                  </Button>
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    onClick={disableProbe}
                  >
                    No health check
                  </Button>
                </div>
                <FieldDescription>
                  A preset also fills a working interval, timeout and failure
                  threshold.
                </FieldDescription>
              </Field>
              {customPath ? (
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
              ) : null}
            </>
          )}
          <AdvancedFields
            fieldPrefix={fieldPrefix}
            control={control}
            register={register}
            formState={formState}
            useExec={value.useExec}
            https={value.https}
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

function AdvancedFields({
  fieldPrefix,
  control,
  register,
  formState,
  useExec,
  https,
}: Pick<
  ProbeFieldsProps,
  'fieldPrefix' | 'control' | 'register' | 'formState'
> & {
  useExec: boolean
  https: boolean
}) {
  const errors = formState.errors[fieldPrefix]
  return (
    <Collapsible className="rounded-md border border-border/60">
      <CollapsibleTrigger className="group flex w-full items-center gap-1.5 px-2.5 py-2 text-left text-sm font-medium text-muted-foreground hover:text-foreground">
        <CaretRightIcon
          className="size-3.5 transition-transform group-data-[panel-open]:rotate-90 motion-reduce:transition-none"
          aria-hidden="true"
        />
        Advanced
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <FieldGroup className="gap-2 px-2.5 pb-2.5">
          {!useExec ? (
            <>
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
                <FieldLabel htmlFor={`${fieldPrefix}-host`}>
                  Host header
                </FieldLabel>
                <Input
                  id={`${fieldPrefix}-host`}
                  {...register(`${fieldPrefix}.host`)}
                  className="font-mono"
                  placeholder="app.example.com"
                />
                <FieldError errors={[errors?.host]} />
              </Field>
            </>
          ) : null}
          <TimingFields
            fieldPrefix={fieldPrefix}
            register={register}
            formState={formState}
          />
        </FieldGroup>
      </CollapsiblePanel>
    </Collapsible>
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
