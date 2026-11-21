import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import {
  BellRingGlassIcon,
  CircleCheckGlassIcon,
  CirclePlusGlassIcon,
  MailGlassIcon,
  SendGlassIcon,
  SmartphoneGlassIcon,
  TriangleAlertGlassIcon,
  WebhookGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { BrandTile } from './BrandLogoBadge'
import { BrowserPushField } from './BrowserPushField'
import { DescriptorFields } from './DescriptorFields'
import {
  FIELD_RESOURCES,
  fieldDescriptorsQueryOptions,
} from '../queries/fieldDescriptors'
import {
  CHANNEL_KIND_BRAND_ICON,
  CHANNEL_KIND_LABEL,
} from './notificationChannelKind'
import {
  useCreateNotificationChannel,
  useTestNotificationChannel,
} from '../queries/notificationChannels'
import {
  CHANNEL_DEFAULT_VALUES,
  camelToSnake,
  snakeToCamel,
  NOTIFICATION_CHANNEL_KIND_ORDER,
  createChannelSchema,
  resolveNotifyUrl,
  type CreateChannelForm,
} from '../lib/notificationChannelForm'

// The global "connect once" flow: picker with real brand marks, inline
// setup guidance, and a "Send test message" action before save.
export function CreateNotificationChannelDialog() {
  const [open, setOpen] = useState(false)
  const [verified, setVerified] = useState(false)
  const [webpushReady, setWebpushReady] = useState(false)
  const createChannel = useCreateNotificationChannel()
  const testChannel = useTestNotificationChannel()
  const { control, handleSubmit, formState, reset, watch, setValue } =
    useForm<CreateChannelForm>({
      resolver: zodResolver(createChannelSchema),
      defaultValues: CHANNEL_DEFAULT_VALUES,
    })
  const kind = watch('kind')
  const { data: descriptorSet } = useQuery({
    ...fieldDescriptorsQueryOptions(FIELD_RESOURCES.notificationChannel),
    retry: false,
  })
  const descriptors = descriptorSet?.fields ?? []
  const descriptorValues = Object.fromEntries(
    Object.entries(watch()).map(([k, v]) => [camelToSnake(k), v]),
  )
  const descriptorErrors = Object.fromEntries(
    Object.entries(formState.errors).map(([k, e]) => [
      camelToSnake(k),
      e.message,
    ]),
  )
  const notifyUrl = watch('notifyUrl')
  const pushoverUserKey = watch('pushoverUserKey')
  const pushoverApiToken = watch('pushoverApiToken')
  const pagerdutyRoutingKey = watch('pagerdutyRoutingKey')
  const resendApiKey = watch('resendApiKey')
  const resendTo = watch('resendTo')
  const opsgenieApiKey = watch('opsgenieApiKey')
  const hasDestination =
    kind === 'pushover'
      ? Boolean(pushoverUserKey.trim() && pushoverApiToken.trim())
      : kind === 'pagerduty'
        ? Boolean(pagerdutyRoutingKey.trim())
        : kind === 'resend'
          ? Boolean(resendApiKey.trim() && resendTo.trim())
          : kind === 'opsgenie'
            ? Boolean(opsgenieApiKey.trim())
            : kind === 'webpush'
              ? webpushReady
              : Boolean(notifyUrl.trim())

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(CHANNEL_DEFAULT_VALUES)
      setVerified(false)
      setWebpushReady(false)
      createChannel.reset()
      testChannel.reset()
    }
  }

  function handleTest() {
    setVerified(false)
    testChannel.mutate(
      { kind, notify_url: resolveNotifyUrl(watch()) },
      {
        onSuccess: () => {
          setVerified(true)
          toast.add({ title: 'Test notification sent.', type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: 'Test notification failed.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const onSubmit = handleSubmit((values) => {
    createChannel.mutate(
      {
        name: values.name.trim(),
        kind: values.kind,
        notify_url: resolveNotifyUrl(values),
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Channel "${created.name}" connected.`,
            type: 'success',
          })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Connect channel</DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WebhookGlassIcon className="size-5 text-muted-foreground" />
            Connect a channel
          </DialogTitle>
          <DialogDescription>
            Connect once here, then attach it from any app&apos;s deploy
            notifications instead of retyping the URL each time.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          {descriptors.length === 0 ? (
            <Alert variant="destructive">
              <AlertDescription>
                The form fields could not be loaded. Reload the page and try
                again.
              </AlertDescription>
            </Alert>
          ) : (
            <DescriptorFields
              fields={descriptors}
              values={descriptorValues}
              idPrefix="channel"
              errors={descriptorErrors}
              onChange={(key, value) => {
                setValue(snakeToCamel(key) as keyof CreateChannelForm, value, {
                  shouldValidate: formState.isSubmitted,
                })
                setVerified(false)
              }}
              renderField={(f) =>
                f.id === 'kind' ? (
                  <Field key={f.id}>
                    <FieldLabel>Channel type</FieldLabel>
                    <Controller
                      control={control}
                      name="kind"
                      render={({ field }) => (
                        <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                          {NOTIFICATION_CHANNEL_KIND_ORDER.map((option) => {
                            const brandIcon = CHANNEL_KIND_BRAND_ICON[option]
                            const selected = field.value === option
                            return (
                              <button
                                key={option}
                                type="button"
                                aria-pressed={selected}
                                onClick={() => {
                                  field.onChange(option)
                                  setVerified(false)
                                }}
                                className={cn(
                                  'flex flex-col items-center gap-1.5 rounded-lg border p-2.5 text-xs font-medium transition-colors',
                                  selected
                                    ? 'border-primary bg-primary/5 text-foreground'
                                    : 'border-border text-muted-foreground hover:bg-muted',
                                )}
                              >
                                {brandIcon ? (
                                  <BrandTile
                                    name={brandIcon}
                                    label={CHANNEL_KIND_LABEL[option]}
                                    size="sm"
                                  />
                                ) : option === 'email' ? (
                                  <MailGlassIcon
                                    className="size-6"
                                    aria-hidden="true"
                                  />
                                ) : option === 'pushover' ? (
                                  <BellRingGlassIcon
                                    className="size-6"
                                    aria-hidden="true"
                                  />
                                ) : option === 'webpush' ? (
                                  <SmartphoneGlassIcon
                                    className="size-6"
                                    aria-hidden="true"
                                  />
                                ) : (
                                  <WebhookGlassIcon
                                    className="size-6"
                                    aria-hidden="true"
                                  />
                                )}
                                <span className="truncate">
                                  {CHANNEL_KIND_LABEL[option]}
                                </span>
                              </button>
                            )
                          })}
                        </div>
                      )}
                    />
                  </Field>
                ) : undefined
              }
            />
          )}
          {kind === 'webpush' ? (
            <BrowserPushField onReadyChange={setWebpushReady} />
          ) : null}

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={testChannel.isPending || !hasDestination}
              onClick={handleTest}
            >
              <SendGlassIcon className="size-5" aria-hidden="true" />
              {testChannel.isPending ? 'Sending...' : 'Send test message'}
            </Button>
            {verified ? (
              <span className="flex items-center gap-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                <CircleCheckGlassIcon
                  className="size-5 text-tone-success"
                  aria-hidden="true"
                />
                Test sent
              </span>
            ) : null}
          </div>

          {createChannel.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{createChannel.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createChannel.isPending}>
              <CirclePlusGlassIcon className="size-5" aria-hidden="true" />
              {createChannel.isPending ? 'Connecting...' : 'Connect channel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
