import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { CpuIcon } from '@phosphor-icons/react/dist/ssr'
import type { DatabaseResource, ServiceResources } from '../types/databaseDetail'
import { useSetDatabaseResources } from '../queries/databases'
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
import { toast } from '@/components/ui/toast'

// Database counterpart to ResourceLimitsEditor.tsx (apps): same schema,
// same lossless-MiB-integer field choice (see that component's own
// top-of-file comment for the float-drift reasoning), same toggle
// layout, same swap/cpuset fields and cross-field validation. The one
// real difference is the submit path: apps go through useUpdateApp's
// general PUT /apps/{name}, databases through the dedicated
// useSetDatabaseResources (PUT /databases/{name}/resources), since
// there is no PUT /databases/{name} to fold this into.
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
      } else if (data.memoryEnabled) {
        // Docker's MemorySwap is memory+swap combined, not swap on top
        // of memory, so a value below the memory limit is nonsensical.
        const memoryN = Number(data.memoryMib)
        if (Number.isFinite(memoryN) && n < memoryN) {
          ctx.addIssue({
            code: 'custom',
            message:
              'Swap is memory+swap combined, so it must be at least the memory limit',
            path: ['swapMib'],
          })
        }
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

// A limit that is toggled off is sent as 0 rather than omitted, the same
// convention ResourceLimitsEditor.tsx's own toResources documents: this
// endpoint is also a full replace of the resources field (never a
// partial merge), so an explicit 0 is the unambiguous way to clear a
// previously-set limit.
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

export function DatabaseResourceLimitsEditor({
  database,
}: {
  database: DatabaseResource
}) {
  const setResources = useSetDatabaseResources()
  const { control, register, handleSubmit, formState } =
    useForm<ResourcesFormValues>({
      resolver: zodResolver(resourcesSchema),
      values: toFieldValues(database.resources),
      resetOptions: { keepDirtyValues: true },
    })

  const onSubmit = handleSubmit((values) => {
    setResources.mutate(
      { name: database.name, resources: toResources(values) },
      {
        onSuccess: () => {
          toast.add({ title: 'Resource limits saved.', type: 'success' })
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
                      id="db-memory-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="db-memory-enabled">
                  Memory limit
                </FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatBytes(database.resources?.memory_bytes)}
              </FieldDescription>
              <Controller
                control={control}
                name="memoryEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="db-memory-mib">
                          Limit (MiB)
                        </FieldLabel>
                        <Input
                          id="db-memory-mib"
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
                      id="db-cpu-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="db-cpu-enabled">CPU limit</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatNanoCpus(database.resources?.nano_cpus)}
              </FieldDescription>
              <Controller
                control={control}
                name="cpuEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="db-cpu-cores">
                          Limit (cores)
                        </FieldLabel>
                        <Input
                          id="db-cpu-cores"
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
                      id="db-swap-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="db-swap-enabled">Swap limit</FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently: {formatBytes(database.resources?.swap_memory_bytes)}
              </FieldDescription>
              <Controller
                control={control}
                name="swapEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="db-swap-mib">
                          Limit (MiB)
                        </FieldLabel>
                        <Input
                          id="db-swap-mib"
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
                      id="db-cpuset-enabled"
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  )}
                />
                <FieldLabel htmlFor="db-cpuset-enabled">
                  CPU pinning
                </FieldLabel>
              </Field>
              <FieldDescription className="mt-1">
                Currently:{' '}
                {database.resources?.cpuset_cpus
                  ? database.resources.cpuset_cpus
                  : 'not set'}
              </FieldDescription>
              <Controller
                control={control}
                name="cpusetEnabled"
                render={({ field }) =>
                  field.value ? (
                    <FieldGroup className="mt-3 gap-2">
                      <Field>
                        <FieldLabel htmlFor="db-cpuset-value">
                          CPUs (Docker cpuset-cpus format)
                        </FieldLabel>
                        <Input
                          id="db-cpuset-value"
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
            <Button type="submit" size="sm" disabled={setResources.isPending}>
              {setResources.isPending ? 'Saving...' : 'Save resource limits'}
            </Button>
          </div>
          {setResources.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{setResources.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
