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
const portSchema = z.object({
  port: z.coerce
    .number({ error: 'Port is required' })
    .int('Port must be a whole number')
    .positive('Port must be a positive integer'),
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
    values: { port: app.port },
    resetOptions: { keepDirtyValues: true },
  })

  const onSubmit = handleSubmit((values) => {
    updateApp.mutate(
      { ...app, port: values.port },
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
          The port the container listens on. Changing it redeploys the app so
          the reconciler can reconcile the running container to the new
          value.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field className="max-w-40">
            <FieldLabel htmlFor="app-port">Port</FieldLabel>
            <Input
              id="app-port"
              inputMode="numeric"
              {...register('port')}
            />
            <FieldError errors={[formState.errors.port]} />
          </Field>

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
