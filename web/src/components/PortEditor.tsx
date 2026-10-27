import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PlugsIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useTranslation } from 'react-i18next'
import {
  formatPublishedPortsInput,
  parsePublishedPorts,
} from '../lib/publishedPortsInput'
import { PublishedPortError } from './PublishedPortError'
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
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'

// BIND_ADDRESS_MODES mirrors internal/bindaddr's Private/Public shorthand
// plus a third, UI-only "custom" mode for a literal IP (a specific host
// interface, or a future WireGuard mesh peer address): the wire value for
// that mode is whatever the operator typed into customBindAddress below,
// never the literal string "custom" itself.
const BIND_ADDRESS_MODES = ['private', 'public', 'custom'] as const
type BindAddressMode = (typeof BIND_ADDRESS_MODES)[number]

const BIND_ADDRESS_LABELS: Record<BindAddressMode, string> = {
  private: 'Private (127.0.0.1)',
  public: 'Public (0.0.0.0)',
  custom: 'Custom IP',
}

// Same coerce/int/positive rule CreateAppFromGitFields/CreateAppFields use
// at creation time, so a port valid at create time stays valid to edit.
// hostPort mirrors CreateAppFields' own replicas field: a plain trimmed
// string, not z.coerce.number, since blank has a real, distinct meaning
// here ("auto-assign") that a coerced number can't represent (Number('')
// is 0, a real port number, not "unset").
const portSchema = z
  .object({
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
    bindAddressMode: z.enum(BIND_ADDRESS_MODES),
    customBindAddress: z.string().trim(),
    publishedPorts: z.string().trim(),
  })
  .refine(
    (data) =>
      data.bindAddressMode !== 'custom' || data.customBindAddress !== '',
    { message: 'Enter an IP address', path: ['customBindAddress'] },
  )

type PortFormInput = z.input<typeof portSchema>
type PortFormOutput = z.output<typeof portSchema>

// toBindAddressFields maps AppDetail.bind_address's wire shape ("private",
// "public", or a literal IP) onto the form's own mode/customBindAddress
// split: any value other than the two known shorthands is treated as a
// literal IP already in place, so editing an app created via app.yaml or
// the CLI with an explicit IP shows that IP back, not a silently reset
// "Private".
function toBindAddressFields(bindAddress: string): {
  bindAddressMode: BindAddressMode
  customBindAddress: string
} {
  if (bindAddress === 'private' || bindAddress === 'public') {
    return { bindAddressMode: bindAddress, customBindAddress: '' }
  }
  return { bindAddressMode: 'custom', customBindAddress: bindAddress }
}

function toFieldValues(app: AppDetail): PortFormInput {
  return {
    port: app.port,
    hostPort: app.host_port ? String(app.host_port) : '',
    publishedPorts: formatPublishedPortsInput(app.published_ports),
    ...toBindAddressFields(app.bind_address),
  }
}

// Same full-replace-PUT pattern as DomainEditor/EnvEditor/
// ResourceLimitsEditor/HealthCheckEditor/DeployStrategyEditor, bound to
// AppDetail.port. `values` + `resetOptions.keepDirtyValues` preserved for
// the same reason: this editor sits on the same AppDetail prop as the
// others and must not let a background refetch stomp unsaved edits in a
// sibling editor.
export function PortEditor({ app }: { app: AppDetail }) {
  const { t } = useTranslation('https')
  const updateApp = useUpdateApp(app.name)
  const { control, register, handleSubmit, watch, setError, formState } =
    useForm<PortFormInput, unknown, PortFormOutput>({
      resolver: zodResolver(portSchema),
      values: toFieldValues(app),
      resetOptions: { keepDirtyValues: true },
    })

  const bindAddressMode = watch('bindAddressMode')

  const onSubmit = handleSubmit((values) => {
    const parsed = parsePublishedPorts(values.publishedPorts)
    if (!parsed.ok) {
      setError('publishedPorts', {
        message: t(`portEditor.errors.${parsed.error.kind}`, {
          entry: parsed.error.entry,
        }),
      })
      return
    }
    const bindAddress =
      values.bindAddressMode === 'custom'
        ? values.customBindAddress
        : values.bindAddressMode
    updateApp.mutate(
      {
        ...app,
        port: values.port,
        host_port: values.hostPort === '' ? null : Number(values.hostPort),
        bind_address: bindAddress,
        published_ports: parsed.ports.length > 0 ? parsed.ports : undefined,
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
          The port the container listens on, which host port Docker binds it to,
          and which network interface that host port is reachable from. Changing
          any of these redeploys the app so the reconciler can reconcile the
          running container to the new value.
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
            <Field
              className="max-w-40"
              data-invalid={formState.errors.port ? true : undefined}
            >
              <FieldLabel htmlFor="app-port">Port</FieldLabel>
              <Input
                id="app-port"
                inputMode="numeric"
                {...register('port')}
                aria-invalid={!!formState.errors.port}
              />
              <FieldError errors={[formState.errors.port]} />
            </Field>

            <Field
              className="max-w-40"
              data-invalid={formState.errors.hostPort ? true : undefined}
            >
              <FieldLabel htmlFor="app-host-port">Host port</FieldLabel>
              <Input
                id="app-host-port"
                inputMode="numeric"
                placeholder="Automatic"
                {...register('hostPort')}
                aria-invalid={!!formState.errors.hostPort}
              />
              <FieldError errors={[formState.errors.hostPort]} />
            </Field>

            <Field className="max-w-56">
              <FieldLabel htmlFor="app-bind-address">
                Network interface
              </FieldLabel>
              <Controller
                control={control}
                name="bindAddressMode"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="app-bind-address" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {BIND_ADDRESS_MODES.map((value) => (
                        <SelectItem key={value} value={value}>
                          {BIND_ADDRESS_LABELS[value]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
              <FieldDescription>
                Private is reachable only from this host. Public exposes it to
                any network that can reach this host.
              </FieldDescription>
            </Field>

            <Field
              className="max-w-72"
              data-invalid={formState.errors.publishedPorts ? true : undefined}
            >
              <FieldLabel htmlFor="app-published-ports">
                {t('portEditor.publishedPorts')}
              </FieldLabel>
              <Input
                id="app-published-ports"
                placeholder={t('portEditor.publishedPortsPlaceholder')}
                spellCheck={false}
                {...register('publishedPorts')}
                aria-invalid={!!formState.errors.publishedPorts}
              />
              <FieldDescription>
                {t('portEditor.publishedPortsHint')}
              </FieldDescription>
              <FieldError errors={[formState.errors.publishedPorts]} />
            </Field>

            {bindAddressMode === 'custom' ? (
              <Field
                className="max-w-48"
                data-invalid={
                  formState.errors.customBindAddress ? true : undefined
                }
              >
                <FieldLabel htmlFor="app-custom-bind-address">
                  IP address
                </FieldLabel>
                <Input
                  id="app-custom-bind-address"
                  placeholder="10.0.0.5"
                  {...register('customBindAddress')}
                  aria-invalid={!!formState.errors.customBindAddress}
                />
                <FieldError errors={[formState.errors.customBindAddress]} />
              </Field>
            ) : null}
          </div>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save port'}
            </Button>
          </div>
          {updateApp.isError ? (
            <PublishedPortError error={updateApp.error} />
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
