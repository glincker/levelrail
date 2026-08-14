import { zodResolver } from '@hookform/resolvers/zod'
import {
  Controller,
  useForm,
  type Control,
  type FormState,
  type UseFormRegister,
} from 'react-hook-form'
import { z } from 'zod'
import { HeartbeatIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail, ServiceProbe } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { formatDurationNs } from '../lib/format'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

// A probe is either not configured (`null` on the wire, ServiceHealth's own
// type) or fully configured with a path and optional interval/timeout/
// failure fields. The form mirrors that with an `enabled` toggle rather
// than treating an empty path as "not configured", so the on/off state is
// explicit instead of inferred from field contents.
const probeSchema = z
  .object({
    enabled: z.boolean(),
    path: z.string().trim(),
    intervalSeconds: z.string().trim(),
    timeoutSeconds: z.string().trim(),
    failures: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (!data.enabled) {
      return
    }
    if (!data.path) {
      ctx.addIssue({
        code: 'custom',
        message: 'Path is required',
        path: ['path'],
      })
    } else if (!data.path.startsWith('/')) {
      ctx.addIssue({
        code: 'custom',
        message: 'Path must start with /',
        path: ['path'],
      })
    }
    if (data.intervalSeconds !== '') {
      const n = Number(data.intervalSeconds)
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Interval must be a positive number of seconds',
          path: ['intervalSeconds'],
        })
      }
    }
    if (data.timeoutSeconds !== '') {
      const n = Number(data.timeoutSeconds)
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Timeout must be a positive number of seconds',
          path: ['timeoutSeconds'],
        })
      }
    }
    if (data.failures !== '') {
      const n = Number(data.failures)
      if (!Number.isInteger(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Failure threshold must be a positive whole number',
          path: ['failures'],
        })
      }
    }
  })

const healthSchema = z.object({
  readiness: probeSchema,
  liveness: probeSchema,
})

type HealthFormValues = z.infer<typeof healthSchema>
type ProbeFormValues = HealthFormValues['readiness']

// interval/timeout are nanosecond time.Duration values on the wire
// (ServiceProbe's own doc comment in appDetail.ts). The form collects
// seconds, the same unit app.yaml's own health.readiness.interval: 5s
// example uses in the app spec and the same unit formatDurationNs
// converts back to for display, so a value round-trips as the same
// number the operator typed rather than drifting through an
// intermediate unit.
function toProbeFieldValues(probe?: ServiceProbe | null): ProbeFormValues {
  return {
    enabled: !!probe,
    path: probe?.path ?? '',
    intervalSeconds:
      probe?.interval && probe.interval > 0
        ? String(probe.interval / 1_000_000_000)
        : '',
    timeoutSeconds:
      probe?.timeout && probe.timeout > 0
        ? String(probe.timeout / 1_000_000_000)
        : '',
    failures:
      probe?.failures && probe.failures > 0 ? String(probe.failures) : '',
  }
}

function toProbe(values: ProbeFormValues): ServiceProbe | null {
  if (!values.enabled) {
    return null
  }
  const probe: ServiceProbe = { path: values.path.trim() }
  if (values.intervalSeconds !== '') {
    probe.interval = Math.round(Number(values.intervalSeconds) * 1_000_000_000)
  }
  if (values.timeoutSeconds !== '') {
    probe.timeout = Math.round(Number(values.timeoutSeconds) * 1_000_000_000)
  }
  if (values.failures !== '') {
    probe.failures = Math.round(Number(values.failures))
  }
  return probe
}

// Same add/remove-list-free, full-replace-PUT pattern as DomainEditor and
// EnvEditor, bound to AppDetail.health instead. `values` +
// `resetOptions.keepDirtyValues` is preserved for the same reason
// DomainEditor documents: this editor, DomainEditor, EnvEditor, and
// ResourceLimitsEditor all read from and write to the same AppDetail
// object, so a background refetch triggered by one of them must not wipe
// out unsaved edits sitting in this one.
export function HealthCheckEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, formState } =
    useForm<HealthFormValues>({
      resolver: zodResolver(healthSchema),
      values: {
        readiness: toProbeFieldValues(app.health?.readiness),
        liveness: toProbeFieldValues(app.health?.liveness),
      },
      resetOptions: { keepDirtyValues: true },
    })

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      {
        ...app,
        health: {
          readiness: toProbe(values.readiness),
          liveness: toProbe(values.liveness),
        },
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Health checks saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HeartbeatIcon className="size-4" />
          Health checks
        </CardTitle>
        <CardDescription>
          Readiness and liveness probes the reconciler uses during rolling
          deploys and to detect a crashed container.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-6"
        >
          <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
            <ProbeFields
              title="Readiness probe"
              fieldPrefix="readiness"
              control={control}
              register={register}
              formState={formState}
              currentProbe={app.health?.readiness}
            />
            <ProbeFields
              title="Liveness probe"
              fieldPrefix="liveness"
              control={control}
              register={register}
              formState={formState}
              currentProbe={app.health?.liveness}
            />
          </div>
          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save health checks'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}

function ProbeFields({
  title,
  fieldPrefix,
  control,
  register,
  formState,
  currentProbe,
}: {
  title: string
  fieldPrefix: 'readiness' | 'liveness'
  control: Control<HealthFormValues>
  register: UseFormRegister<HealthFormValues>
  formState: FormState<HealthFormValues>
  currentProbe?: ServiceProbe | null
}) {
  const errors = formState.errors[fieldPrefix]
  const currentSummary = currentProbe
    ? `Currently: ${currentProbe.path}, every ${formatDurationNs(currentProbe.interval)}`
    : 'Currently: not configured'

  return (
    <div className="rounded-md border border-border p-3">
      <Field orientation="horizontal">
        <Controller
          control={control}
          name={`${fieldPrefix}.enabled`}
          render={({ field }) => (
            <Switch
              id={`${fieldPrefix}-enabled`}
              checked={field.value}
              onCheckedChange={field.onChange}
            />
          )}
        />
        <FieldLabel htmlFor={`${fieldPrefix}-enabled`}>{title}</FieldLabel>
      </Field>
      <FieldDescription className="mt-1">{currentSummary}</FieldDescription>

      <Controller
        control={control}
        name={`${fieldPrefix}.enabled`}
        render={({ field }) =>
          field.value ? (
            <FieldGroup className="mt-3 gap-2">
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
            </FieldGroup>
          ) : (
            <p className="mt-3 text-sm text-muted-foreground">
              Probe is not configured.
            </p>
          )
        }
      />
    </div>
  )
}
