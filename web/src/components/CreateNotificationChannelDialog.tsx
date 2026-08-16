import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
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
import type { NotificationChannelKind } from '../types/notificationChannel'

const KIND_ORDER: NotificationChannelKind[] = [
  'slack',
  'discord',
  'telegram',
  'generic',
  'email',
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
}

const createChannelSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum(['generic', 'slack', 'discord', 'telegram', 'email']),
    notifyUrl: z.string().trim().min(1, 'Destination is required'),
  })
  .superRefine((data, ctx) => {
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
      { kind, notify_url: notifyUrl.trim() },
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
        notify_url: values.notifyUrl.trim(),
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
                <div className="grid grid-cols-5 gap-2">
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

          <Field>
            <FieldLabel htmlFor="channel-notify-url">
              {kind === 'email' ? 'Notify email address' : 'Webhook URL'}
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

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={testChannel.isPending || !notifyUrl.trim()}
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
