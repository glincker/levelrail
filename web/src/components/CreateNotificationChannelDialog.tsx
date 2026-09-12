import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  BellRingingIcon,
  CheckCircleIcon,
  EnvelopeSimpleIcon,
  PaperPlaneTiltIcon,
  PlusCircleIcon,
  WarningIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { BrandIcon } from './BrandIcon'
import {
  CHANNEL_KIND_BRAND_ICON,
  CHANNEL_KIND_LABEL,
} from './notificationChannelKind'
import {
  useCreateNotificationChannel,
  useTestNotificationChannel,
} from '../queries/notificationChannels'
import { buildOpsgenieNotifyUrl } from '../lib/opsgenieNotifyUrl'
import { buildPushoverNotifyUrl } from '../lib/pushoverNotifyUrl'
import { buildResendNotifyUrl } from '../lib/resendNotifyUrl'
import type { NotificationChannelKind } from '../types/notificationChannel'

const KIND_ORDER: NotificationChannelKind[] = [
  'slack',
  'discord',
  'telegram',
  'teams',
  'mattermost',
  'lark',
  'rocketchat',
  'webex',
  'googlechat',
  'generic',
  'email',
  'pushover',
  'pagerduty',
  'opsgenie',
  'resend',
  'ntfy',
  'gotify',
]

// Destination placeholder and setup-guide link per kind: each URL is the
// platform's own official webhook/bot setup doc.
const KIND_META: Record<
  NotificationChannelKind,
  { placeholder: string; href?: string }
> = {
  slack: {
    placeholder: 'https://hooks.slack.com/services/...',
    href: 'https://api.slack.com/messaging/webhooks',
  },
  discord: {
    placeholder: 'https://discord.com/api/webhooks/...',
    href: 'https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks',
  },
  telegram: {
    placeholder: 'https://api.telegram.org/bot<token>/sendMessage?chat_id=...',
    href: 'https://core.telegram.org/bots/tutorial',
  },
  generic: { placeholder: 'https://example.com/webhook' },
  email: { placeholder: 'ops@example.com' },
  pushover: {
    placeholder: '',
    href: 'https://pushover.net/api#registration',
  },
  pagerduty: {
    placeholder: '',
    href: 'https://support.pagerduty.com/docs/services-and-integrations#events-api-v2',
  },
  teams: {
    placeholder: 'https://example.webhook.office.com/webhookb2/...',
    href: 'https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook',
  },
  mattermost: {
    placeholder: 'https://mattermost.example.com/hooks/...',
    href: 'https://developers.mattermost.com/integrate/webhooks/incoming/',
  },
  lark: {
    placeholder: 'https://open.larksuite.com/open-apis/bot/v2/hook/...',
    href: 'https://open.larksuite.com/document/client-docs/bot-v3/add-custom-bot',
  },
  resend: { placeholder: '' },
  ntfy: {
    placeholder: 'https://ntfy.sh/my-topic',
    href: 'https://docs.ntfy.sh/publish/',
  },
  gotify: {
    placeholder: 'https://gotify.example.com/message?token=...',
    href: 'https://gotify.net/docs/pushmsg',
  },
  rocketchat: {
    placeholder: 'https://rocketchat.example.com/hooks/...',
    href: 'https://docs.rocket.chat/docs/integrations#incoming-webhook-script',
  },
  opsgenie: { placeholder: '' },
  webex: {
    placeholder: 'https://webexapis.com/v1/webhooks/incoming/...',
    href: 'https://developer.webex.com/messaging/docs/api/guides/webhooks',
  },
  googlechat: {
    placeholder: 'https://chat.googleapis.com/v1/spaces/.../messages?key=...',
    href: 'https://developers.google.com/workspace/chat/quickstart/webhooks',
  },
}

const createChannelSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum([
      'generic',
      'slack',
      'discord',
      'telegram',
      'email',
      'pushover',
      'pagerduty',
      'teams',
      'resend',
      'ntfy',
      'gotify',
      'mattermost',
      'lark',
      'rocketchat',
      'opsgenie',
      'webex',
      'googlechat',
    ]),
    notifyUrl: z.string().trim(),
    pushoverUserKey: z.string().trim(),
    pushoverApiToken: z.string().trim(),
    pagerdutyRoutingKey: z.string().trim(),
    resendApiKey: z.string().trim(),
    resendTo: z.string().trim(),
    resendFrom: z.string().trim(),
    opsgenieApiKey: z.string().trim(),
  })
  .superRefine((data, ctx) => {
    if (data.kind === 'pushover') {
      if (!data.pushoverUserKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'User Key is required',
          path: ['pushoverUserKey'],
        })
      }
      if (!data.pushoverApiToken) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Token is required',
          path: ['pushoverApiToken'],
        })
      }
      return
    }
    if (data.kind === 'pagerduty') {
      if (!data.pagerdutyRoutingKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'Integration/Routing Key is required',
          path: ['pagerdutyRoutingKey'],
        })
      }
      return
    }
    if (data.kind === 'resend') {
      if (!data.resendApiKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Key is required',
          path: ['resendApiKey'],
        })
      }
      if (!data.resendTo) {
        ctx.addIssue({
          code: 'custom',
          message: 'Destination address is required',
          path: ['resendTo'],
        })
      } else if (!z.email().safeParse(data.resendTo).success) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a valid email address',
          path: ['resendTo'],
        })
      }
      if (data.resendFrom && !z.email().safeParse(data.resendFrom).success) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a valid email address',
          path: ['resendFrom'],
        })
      }
      return
    }
    if (data.kind === 'opsgenie') {
      if (!data.opsgenieApiKey) {
        ctx.addIssue({
          code: 'custom',
          message: 'API Key is required',
          path: ['opsgenieApiKey'],
        })
      }
      return
    }
    if (!data.notifyUrl) {
      ctx.addIssue({
        code: 'custom',
        message: 'Destination is required',
        path: ['notifyUrl'],
      })
      return
    }
    const isValid =
      data.kind === 'email'
        ? z.email().safeParse(data.notifyUrl).success
        : z.url().safeParse(data.notifyUrl).success
    if (!isValid) {
      ctx.addIssue({
        code: 'custom',
        message:
          data.kind === 'email'
            ? 'Must be a valid email address'
            : 'Must be a valid URL',
        path: ['notifyUrl'],
      })
    }
  })

type CreateChannelForm = z.infer<typeof createChannelSchema>

const DEFAULT_VALUES: CreateChannelForm = {
  name: '',
  kind: 'slack',
  notifyUrl: '',
  pushoverUserKey: '',
  pushoverApiToken: '',
  pagerdutyRoutingKey: '',
  resendApiKey: '',
  resendTo: '',
  resendFrom: '',
  opsgenieApiKey: '',
}

// The single notify_url string the API expects, per kind: Pushover's and
// Resend's credentials get packed into it client-side
// (buildPushoverNotifyUrl/buildResendNotifyUrl), PagerDuty's routing key
// is sent through as-is (it's the whole notify_url for that kind, not a
// URL), every other kind sends the destination field as-is.
function resolveNotifyUrl(values: CreateChannelForm): string {
  if (values.kind === 'pushover') {
    return buildPushoverNotifyUrl(
      values.pushoverUserKey.trim(),
      values.pushoverApiToken.trim(),
    )
  }
  if (values.kind === 'pagerduty') {
    return values.pagerdutyRoutingKey.trim()
  }
  if (values.kind === 'resend') {
    return buildResendNotifyUrl(
      values.resendApiKey.trim(),
      values.resendTo.trim(),
      values.resendFrom.trim() || undefined,
    )
  }
  if (values.kind === 'opsgenie') {
    return buildOpsgenieNotifyUrl(values.opsgenieApiKey.trim())
  }
  return values.notifyUrl.trim()
}

// The global "connect once" flow: picker with real brand marks, inline
// setup guidance, and a "Send test message" action before save.
export function CreateNotificationChannelDialog() {
  const [open, setOpen] = useState(false)
  const [verified, setVerified] = useState(false)
  const createChannel = useCreateNotificationChannel()
  const testChannel = useTestNotificationChannel()
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<CreateChannelForm>({
      resolver: zodResolver(createChannelSchema),
      defaultValues: DEFAULT_VALUES,
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
            : Boolean(notifyUrl.trim())

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(DEFAULT_VALUES)
      setVerified(false)
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
            <WebhooksLogoIcon className="size-4 text-muted-foreground" />
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
                  {KIND_ORDER.map((option) => {
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
                          <BrandIcon name={brandIcon} className="size-5" />
                        ) : option === 'email' ? (
                          <EnvelopeSimpleIcon
                            className="size-5"
                            aria-hidden="true"
                          />
                        ) : option === 'pushover' ? (
                          <BellRingingIcon
                            className="size-5"
                            aria-hidden="true"
                          />
                        ) : (
                          <WebhooksLogoIcon
                            className="size-5"
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
                <FieldHint href={KIND_META.pushover.href}>
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
              <FieldHint href={KIND_META.pagerduty.href}>
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
                placeholder={KIND_META[kind].placeholder}
                {...register('notifyUrl', {
                  onChange: () => {
                    setVerified(false)
                  },
                })}
              />
              <FieldError errors={[formState.errors.notifyUrl]} />
              <FieldHint href={KIND_META[kind].href}>
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
              <PaperPlaneTiltIcon className="size-3.5" aria-hidden="true" />
              {testChannel.isPending ? 'Sending...' : 'Send test message'}
            </Button>
            {verified ? (
              <span className="flex items-center gap-1 text-xs font-medium text-emerald-600 dark:text-emerald-400">
                <CheckCircleIcon className="size-3.5" aria-hidden="true" />
                Test sent
              </span>
            ) : null}
          </div>

          {createChannel.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{createChannel.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createChannel.isPending}>
              <PlusCircleIcon className="size-3.5" aria-hidden="true" />
              {createChannel.isPending ? 'Connecting...' : 'Connect channel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
