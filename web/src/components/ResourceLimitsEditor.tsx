import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { CpuIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail, ServiceResources } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { formatBytes, formatNanoCpus } from '../lib/format'
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
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'

// Unit choice: memory is collected as a single whole-number MiB field
// rather than a "value + unit" pair. formatBytes (lib/format.ts) already
// steps through B/KiB/MiB/GiB/TiB for display, but round-tripping through
// a unit picker here would risk exactly the lossy-conversion problem the
// task called out (entering "0.5" GiB and saving something like
// "536870912.0000001" bytes from float drift). A single MiB integer
// field multiplies by 1_048_576 (2^20) with no fractional step, so
// memoryMib -> memory_bytes -> memoryMib is exact in both directions,
// and formatBytes of whatever gets saved always prints a round number.
// Larger apps that want GiB just type the MiB equivalent (e.g. 4096 for
// 4 GiB); that is a fair tradeoff for staying lossless.
// cpusetPattern mirrors internal/spec/schema/app.schema.json's own
// resources.cpuSet pattern exactly: Docker's cpuset-cpus format, e.g.
// "0-3" or "0,2".
const cpusetPattern = /^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$/

const resourcesSchema = z
  .object({
    memoryEnabled: z.boolean(),
    memoryMib: z.string().trim(),
    cpuEnabled: z.boolean(),
    cpuCores: z.string().trim(),
    swapEnabled: z.boolean(),
    swapMib: z.string().trim(),
    cpusetEnabled: z.boolean(),
    cpusetValue: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.memoryEnabled) {
      const n = Number(data.memoryMib)
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Enter a positive whole number of MiB',
          path: ['memoryMib'],
        })
      } else if (!Number.isInteger(n)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Memory must be a whole number of MiB',
          path: ['memoryMib'],
        })
      }
    }
    if (data.cpuEnabled) {
      const n = Number(data.cpuCores)
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Enter a positive number of CPU cores, e.g. 0.5',
          path: ['cpuCores'],
        })
      }
    }
    if (data.swapEnabled) {
      if (!data.memoryEnabled) {
        ctx.addIssue({
          code: 'custom',
          message: 'Swap requires a memory limit to also be set',
          path: ['swapMib'],
        })
      }
      const n = Number(data.swapMib)
      if (!Number.isFinite(n) || n <= 0) {
        ctx.addIssue({
          code: 'custom',
          message: 'Enter a positive whole number of MiB',
          path: ['swapMib'],
        })
      } else if (!Number.isInteger(n)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Swap must be a whole number of MiB',
          path: ['swapMib'],
        })
      }
    }
    if (data.cpusetEnabled && !cpusetPattern.test(data.cpusetValue)) {
      ctx.addIssue({
        code: 'custom',
        message: 'Enter a CPU list/range, e.g. "0-3" or "0,2"',
        path: ['cpusetValue'],
      })
    }
  })

type ResourcesFormValues = z.infer<typeof resourcesSchema>

function toFieldValues(
  resources?: ServiceResources | null,
): ResourcesFormValues {
  const memoryBytes = resources?.memory_bytes
  const nanoCpus = resources?.nano_cpus
  const swapBytes = resources?.swap_memory_bytes
  const cpusetCpus = resources?.cpuset_cpus
  return {
    memoryEnabled: !!memoryBytes && memoryBytes > 0,
    memoryMib:
      memoryBytes && memoryBytes > 0
        ? String(Math.round(memoryBytes / 1_048_576))
        : '',
    cpuEnabled: !!nanoCpus && nanoCpus > 0,
    cpuCores: nanoCpus && nanoCpus > 0 ? String(nanoCpus / 1_000_000_000) : '',
    swapEnabled: !!swapBytes && swapBytes > 0,
    swapMib:
      swapBytes && swapBytes > 0
        ? String(Math.round(swapBytes / 1_048_576))
        : '',
    cpusetEnabled: !!cpusetCpus,
    cpusetValue: cpusetCpus ?? '',
  }
}

// A limit that is toggled off is sent as 0 rather than omitted, matching
// formatBytes/formatNanoCpus's own `<= 0` check (lib/format.ts) for what
// "not set" means on this wire shape: since PUT is a full replace, an
// explicit 0 is the unambiguous way to clear a previously-set limit
// rather than relying on JSON key omission producing the same zero value.
function toResources(values: ResourcesFormValues): ServiceResources {
  return {
    memory_bytes: values.memoryEnabled
      ? Math.round(Number(values.memoryMib)) * 1_048_576
      : 0,
    nano_cpus: values.cpuEnabled
      ? Math.round(Number(values.cpuCores) * 1_000_000_000)
      : 0,
    swap_memory_bytes: values.swapEnabled
      ? Math.round(Number(values.swapMib)) * 1_048_576
      : 0,
    cpuset_cpus: values.cpusetEnabled ? values.cpusetValue : '',
  }
}

// Same full-replace-PUT pattern as DomainEditor/EnvEditor/
// HealthCheckEditor, bound to AppDetail.resources. `values` +
// `resetOptions.keepDirtyValues` preserved for the same reason: this
// editor sits on the same AppDetail prop as the others and must not let a
// background refetch stomp unsaved edits in a sibling editor.
export function ResourceLimitsEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const notifyRestartRequired = useRestartRequiredToast()
  const { control, register, handleSubmit, formState } =
    useForm<ResourcesFormValues>({
      resolver: zodResolver(resourcesSchema),
      values: toFieldValues(app.resources),
      resetOptions: { keepDirtyValues: true },
    })

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      { ...app, resources: toResources(values) },
      {
        onSuccess: () => {
          notifyRestartRequired(app.name, 'Resource limits saved.')
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CpuIcon className="size-4" />
          Resource limits
        </CardTitle>
        <CardDescription>
          Memory, CPU, swap, and CPU pinning limits applied to the container
          at create time. Leave a limit off to run unbounded.
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
            <div className="rounded-md border border-border p-3">
              <Field orientation="horizontal">
                <Controller
                  control={control}
                  name="memoryEnabled"
                  render={({ field }) => (
                    <Switch
                      id="memory-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="memory-enabled">Memory limit</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatBytes(app.resources?.memory_bytes)}
              </FieldDescription>
              <Controller
                control={control}
                name="memoryEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="memory-mib">
                          Limit (MiB)
                        </FieldLabel>
                        <Input
                          id="memory-mib"
                          {...register('memoryMib')}
                          inputMode="numeric"
                          placeholder="512"
                        />
                        <FieldError errors={[formState.errors.memoryMib]} />
                      </Field>
                    </FieldGroup>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">
                      No memory limit set.
                    </p>
                  )
                }
              />
            </div>

            <div className="rounded-md border border-border p-3">
              <Field orientation="horizontal">
                <Controller
                  control={control}
                  name="cpuEnabled"
                  render={({ field }) => (
                    <Switch
                      id="cpu-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="cpu-enabled">CPU limit</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatNanoCpus(app.resources?.nano_cpus)}
              </FieldDescription>
              <Controller
                control={control}
                name="cpuEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="cpu-cores">
                          Limit (cores)
                        </FieldLabel>
                        <Input
                          id="cpu-cores"
                          {...register('cpuCores')}
                          inputMode="decimal"
                          placeholder="0.5"
                        />
                        <FieldError errors={[formState.errors.cpuCores]} />
                      </Field>
                    </FieldGroup>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">
                      No CPU limit set.
                    </p>
                  )
                }
              />
            </div>

            <div className="rounded-md border border-border p-3">
              <Field orientation="horizontal">
                <Controller
                  control={control}
                  name="swapEnabled"
                  render={({ field }) => (
                    <Switch
                      id="swap-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="swap-enabled">Swap limit</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatBytes(app.resources?.swap_memory_bytes)}
              </FieldDescription>
              <Controller
                control={control}
                name="swapEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="swap-mib">Limit (MiB)</FieldLabel>
                        <Input
                          id="swap-mib"
                          {...register('swapMib')}
                          inputMode="numeric"
                          placeholder="1024"
                        />
                        <FieldError errors={[formState.errors.swapMib]} />
                      </Field>
                    </FieldGroup>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">
                      No swap limit set.
                    </p>
                  )
                }
              />
            </div>

            <div className="rounded-md border border-border p-3">
              <Field orientation="horizontal">
                <Controller
                  control={control}
                  name="cpusetEnabled"
                  render={({ field }) => (
                    <Switch
                      id="cpuset-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="cpuset-enabled">CPU pinning</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently:{' '}
                {app.resources?.cpuset_cpus
                  ? app.resources.cpuset_cpus
                  : 'not set'}
              </FieldDescription>
              <Controller
                control={control}
                name="cpusetEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="cpuset-value">
                          CPUs (Docker cpuset-cpus format)
                        </FieldLabel>
                        <Input
                          id="cpuset-value"
                          {...register('cpusetValue')}
                          placeholder="0-3"
                        />
                        <FieldError errors={[formState.errors.cpusetValue]} />
                      </Field>
                    </FieldGroup>
                  ) : (
                    <p className="mt-3 text-sm text-muted-foreground">
                      No CPU pinning set.
                    </p>
                  )
                }
              />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save resource limits'}
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
