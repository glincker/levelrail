import { useState } from 'react'
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
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { BrandTile } from './BrandLogoBadge'
import { BrowserPushField } from './BrowserPushField'
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
  CHANNEL_KIND_META,
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
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<CreateChannelForm>({
      resolver: zodResolver(createChannelSchema),
      defaultValues: CHANNEL_DEFAULT_VALUES,
    })
  const kind = watch('kind')
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
          <Field>
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

          <Field>
            <FieldLabel htmlFor="channel-name">Name</FieldLabel>
            <Input
              id="channel-name"
              placeholder="e.g. Team Slack"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          {kind === 'pushover' ? (
            <>
              <Field>
                <FieldLabel htmlFor="channel-pushover-user-key">
                  Pushover User Key
                </FieldLabel>
                <Input
                  id="channel-pushover-user-key"
                  placeholder="uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
                  {...register('pushoverUserKey', {
                    onChange: () => {
                      setVerified(false)
                    },
                  })}
                />
                <FieldError errors={[formState.errors.pushoverUserKey]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="channel-pushover-api-token">
                  Pushover Application API Token
                </FieldLabel>
                <Input
                  id="channel-pushover-api-token"
                  placeholder="azGDORePK8gMaC0QOYAMyEEuzJnyUi"
                  {...register('pushoverApiToken', {
                    onChange: () => {
                      setVerified(false)
                    },
                  })}
                />
                <FieldError errors={[formState.errors.pushoverApiToken]} />
                <FieldHint href={CHANNEL_KIND_META.pushover.href}>
                  What gets sent: app name, image/tag, success or failure, and
                  the error message on failure. Nothing else about your app.
                </FieldHint>
              </Field>
            </>
          ) : kind === 'pagerduty' ? (
            <Field>
              <FieldLabel htmlFor="channel-pagerduty-routing-key">
                Integration/Routing Key
              </FieldLabel>
              <Input
                id="channel-pagerduty-routing-key"
                placeholder="a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
                {...register('pagerdutyRoutingKey', {
                  onChange: () => {
                    setVerified(false)
                  },
                })}
              />
              <FieldError errors={[formState.errors.pagerdutyRoutingKey]} />
              <FieldHint href={CHANNEL_KIND_META.pagerduty.href}>
                What gets sent: app name, image/tag, success or failure, and the
                error message on failure. Nothing else about your app.
              </FieldHint>
            </Field>
          ) : kind === 'resend' ? (
            <>
              <Field>
                <FieldLabel htmlFor="channel-resend-api-key">
                  Resend API Key
                </FieldLabel>
                <Input
                  id="channel-resend-api-key"
                  placeholder="re_123456789"
                  {...register('resendApiKey', {
                    onChange: () => {
                      setVerified(false)
                    },
                  })}
                />
                <FieldError errors={[formState.errors.resendApiKey]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="channel-resend-to">
                  Destination email address
                </FieldLabel>
                <Input
                  id="channel-resend-to"
                  placeholder="ops@example.com"
                  {...register('resendTo', {
                    onChange: () => {
                      setVerified(false)
                    },
                  })}
                />
                <FieldError errors={[formState.errors.resendTo]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="channel-resend-from">
                  From address (optional)
                </FieldLabel>
                <Input
                  id="channel-resend-from"
                  placeholder="alerts@yourdomain.com"
                  {...register('resendFrom', {
                    onChange: () => {
                      setVerified(false)
                    },
                  })}
                />
                <FieldError errors={[formState.errors.resendFrom]} />
                <FieldHint href="https://resend.com/docs/dashboard/domains/introduction">
                  Left blank, sends from Resend&apos;s own onboarding@resend.dev
                  sender, which works without verifying a domain.
                </FieldHint>
              </Field>
            </>
          ) : kind === 'opsgenie' ? (
            <Field>
              <FieldLabel htmlFor="channel-opsgenie-api-key">
                Opsgenie API Key
              </FieldLabel>
              <Input
                id="channel-opsgenie-api-key"
                placeholder="00000000-0000-0000-0000-000000000000"
                {...register('opsgenieApiKey', {
                  onChange: () => {
                    setVerified(false)
                  },
                })}
              />
              <FieldError errors={[formState.errors.opsgenieApiKey]} />
              <FieldHint href="https://support.atlassian.com/opsgenie/docs/api-integration/">
                What gets sent: app name, image/tag, success or failure, and the
                error message on failure. Nothing else about your app.
              </FieldHint>
            </Field>
          ) : kind === 'webpush' ? (
            <BrowserPushField onReadyChange={setWebpushReady} />
          ) : (
            <Field>
              <FieldLabel htmlFor="channel-notify-url">
                {kind === 'email'
                  ? 'Notify email address'
                  : kind === 'ntfy'
                    ? 'Topic URL'
                    : kind === 'gotify'
                      ? 'Message endpoint URL'
                      : 'Webhook URL'}
              </FieldLabel>
              <Input
                id="channel-notify-url"
                placeholder={CHANNEL_KIND_META[kind].placeholder}
                {...register('notifyUrl', {
                  onChange: () => {
                    setVerified(false)
                  },
                })}
              />
              <FieldError errors={[formState.errors.notifyUrl]} />
              <FieldHint href={CHANNEL_KIND_META[kind].href}>
                What gets sent: app name, image/tag, success or failure, and the
                error message on failure. Nothing else about your app.
              </FieldHint>
            </Field>
          )}

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
