import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { PlugsIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'

// Same coerce/int/positive rule CreateAppFromGitFields/CreateAppFields use
// at creation time, so a port valid at create time stays valid to edit.
// hostPort mirrors CreateAppFields' own replicas field: a plain trimmed
// string, not z.coerce.number, since blank has a real, distinct meaning
// here ("auto-assign") that a coerced number can't represent (Number('')
// is 0, a real port number, not "unset").
const portSchema = z.object({
  port: z.coerce
    .number({ error: 'Port is required' })
    .int('Port must be a whole number')
    .positive('Port must be a positive integer'),
  hostPort: z
    .string()
    .trim()
    .refine(
      (v) =>
        v === '' || (/^\d+$/.test(v) && Number(v) >= 1 && Number(v) <= 65535),
      'Host port must be between 1 and 65535, or left blank for automatic',
    ),
})

type PortFormInput = z.input<typeof portSchema>
type PortFormOutput = z.output<typeof portSchema>

// Same full-replace-PUT pattern as DomainEditor/EnvEditor/
// ResourceLimitsEditor/HealthCheckEditor/DeployStrategyEditor, bound to
// AppDetail.port. `values` + `resetOptions.keepDirtyValues` preserved for
// the same reason: this editor sits on the same AppDetail prop as the
// others and must not let a background refetch stomp unsaved edits in a
// sibling editor.
export function PortEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const { register, handleSubmit, formState } = useForm<
    PortFormInput,
    unknown,
    PortFormOutput
  >({
    resolver: zodResolver(portSchema),
    values: {
      port: app.port,
      hostPort: app.host_port ? String(app.host_port) : '',
    },
    resetOptions: { keepDirtyValues: true },
  })

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      {
        ...app,
        port: values.port,
        host_port: values.hostPort === '' ? null : Number(values.hostPort),
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Port saved.', type: 'success' })
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugsIcon className="size-4" />
          Port
        </CardTitle>
        <CardDescription>
          The port the container listens on, and optionally which host port
          Docker binds it to. Leave host port blank to let Docker assign one
          automatically. Changing either redeploys the app so the reconciler can
          reconcile the running container to the new value.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <div className="flex flex-wrap gap-4">
            <Field className="max-w-40">
              <FieldLabel htmlFor="app-port">Port</FieldLabel>
              <Input id="app-port" inputMode="numeric" {...register('port')} />
              <FieldError errors={[formState.errors.port]} />
            </Field>

            <Field className="max-w-40">
              <FieldLabel htmlFor="app-host-port">Host port</FieldLabel>
              <Input
                id="app-host-port"
                inputMode="numeric"
                placeholder="Automatic"
                {...register('hostPort')}
              />
              <FieldError errors={[formState.errors.hostPort]} />
            </Field>
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save port'}
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
