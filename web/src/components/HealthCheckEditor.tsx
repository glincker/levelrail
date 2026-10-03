import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import { HeartbeatIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import {
  probeSchema,
  toProbe,
  toProbeFieldValues,
} from '../lib/healthProbeForm'
import { probeTimingDefaults } from '../lib/healthCheckDefaults'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { HelpLink } from '@/components/HelpLink'
import { ProbeFields, type HealthFormValues } from './ProbeFields'
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'

const healthSchema = z.object({
  readiness: probeSchema,
  liveness: probeSchema,
})

// Same add/remove-list-free, full-replace-PUT pattern as DomainEditor and
// EnvEditor, bound to AppDetail.health instead. `values` +
// `resetOptions.keepDirtyValues` is preserved for the same reason
// DomainEditor documents: this editor, DomainEditor, EnvEditor, and
// ResourceLimitsEditor all read from and write to the same AppDetail
// object, so a background refetch triggered by one of them must not wipe
// out unsaved edits sitting in this one.
export function HealthCheckEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const notifyRestartRequired = useRestartRequiredToast()
  const { control, register, handleSubmit, formState, setValue, getValues } =
    useForm<HealthFormValues>({
      resolver: zodResolver(healthSchema),
      values: {
        readiness: toProbeFieldValues(app.health?.readiness),
        liveness: toProbeFieldValues(app.health?.liveness),
      },
      resetOptions: { keepDirtyValues: true },
    })
  const readinessEnabled = useWatch({ control, name: 'readiness.enabled' })

  // The overwhelmingly common case is the same path for both probes, so
  // one click copies readiness's HTTP/exec settings onto liveness,
  // leaving liveness's own timing alone unless it is still blank.
  function copyReadinessToLiveness() {
    const readiness = getValues('readiness')
    const liveness = getValues('liveness')
    const defaults = probeTimingDefaults('liveness')
    setValue(
      'liveness',
      {
        ...liveness,
        enabled: readiness.enabled,
        useExec: readiness.useExec,
        path: readiness.path,
        https: readiness.https,
        host: readiness.host,
        tlsSkipVerify: readiness.tlsSkipVerify,
        followRedirects: readiness.followRedirects,
        expectedStatus: readiness.expectedStatus,
        execCommand: readiness.execCommand,
        intervalSeconds: liveness.intervalSeconds || defaults.intervalSeconds,
        timeoutSeconds: liveness.timeoutSeconds || defaults.timeoutSeconds,
        failures: liveness.failures || defaults.failures,
      },
      { shouldDirty: true, shouldValidate: true },
    )
  }

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      {
        ...app,
        health: {
          ...app.health,
          readiness: toProbe(values.readiness),
          liveness: toProbe(values.liveness),
        },
      },
      {
        onSuccess: () => {
          notifyRestartRequired(app.name, 'Health checks saved.')
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
          <HelpLink
            path="/app-spec-reference#health-checks"
            label="Health check reference"
          />
        </CardTitle>
        <CardDescription>
          Readiness gates a deploy&apos;s cutover; liveness restarts a hung
          container. Each is an HTTP(S) request or a command run inside the
          container.
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
              key={`${app.name}-readiness`}
              title="Readiness probe"
              fieldPrefix="readiness"
              control={control}
              register={register}
              setValue={setValue}
              formState={formState}
              currentProbe={app.health?.readiness}
              appName={app.name}
            />
            <ProbeFields
              key={`${app.name}-liveness`}
              title="Liveness probe"
              fieldPrefix="liveness"
              control={control}
              register={register}
              setValue={setValue}
              formState={formState}
              currentProbe={app.health?.liveness}
              appName={app.name}
              headerAction={
                <Button
                  type="button"
                  size="xs"
                  variant="outline"
                  disabled={!readinessEnabled}
                  onClick={copyReadinessToLiveness}
                >
                  Same as readiness
                </Button>
              }
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
