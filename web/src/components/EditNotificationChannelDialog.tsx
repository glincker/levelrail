import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CheckCircleIcon,
  PaperPlaneTiltIcon,
  PencilSimpleIcon,
  WarningIcon,
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
import { Switch } from '@/components/ui/switch'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { BrandIcon } from './BrandIcon'
import {
  CHANNEL_KIND_BRAND_ICON,
  CHANNEL_KIND_LABEL,
} from './notificationChannelKind'
import {
  useTestNotificationChannel,
  useUpdateNotificationChannel,
} from '../queries/notificationChannels'
import type { NotificationChannel } from '../types/notificationChannel'

const KIND_ORDER = [
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
] as const

const editChannelSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  kind: z.enum(KIND_ORDER),
  notifyUrl: z.string().trim().min(1, 'Destination is required'),
  enabled: z.boolean(),
})

type EditChannelForm = z.infer<typeof editChannelSchema>

// Deliberately simpler than CreateNotificationChannelDialog: that dialog
// decomposes a pushover/resend/opsgenie destination into separate
// credential fields and repacks them into notify_url
// (buildPushoverNotifyUrl and friends), but there is no inverse of that
// packing here to pre-fill those fields back out of an existing
// notify_url. Editing works directly on the one field the server
// actually stores (notify_url), which is also exactly the field the
// motivating bug report (a typo'd webhook URL) needs fixed.
function defaultsFromChannel(channel: NotificationChannel): EditChannelForm {
  return {
    name: channel.name,
    kind: channel.kind,
    notifyUrl: channel.notify_url,
    enabled: channel.enabled,
  }
}

// Edit-via-dialog flow for an existing notification channel: PUT
// /api/v1/notification-channels/{id} is a full replace
// (handleUpdateNotificationChannel's own doc comment), so this form
// starts pre-filled from channel and every field round-trips through the
// same request shape create already uses.
export function EditNotificationChannelDialog({
  channel,
}: {
  channel: NotificationChannel
}) {
  const [open, setOpen] = useState(false)
  const [verified, setVerified] = useState(false)
  const updateChannel = useUpdateNotificationChannel()
  const testChannel = useTestNotificationChannel()
  const defaultValues = defaultsFromChannel(channel)
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<EditChannelForm>({
      resolver: zodResolver(editChannelSchema),
      defaultValues,
    })
  const kind = watch('kind')
  const notifyUrl = watch('notifyUrl')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      setVerified(false)
      updateChannel.reset()
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
    updateChannel.mutate(
      {
        id: channel.id,
        req: {
          name: values.name.trim(),
          kind: values.kind,
          notify_url: values.notifyUrl.trim(),
          enabled: values.enabled,
        },
      },
      {
        onSuccess: (updated) => {
          handleOpenChange(false)
          toast.add({
            title: `Channel "${updated.name}" updated.`,
            type: 'success',
          })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
        Edit
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <PencilSimpleIcon className="size-4 text-muted-foreground" />
            Edit channel
          </DialogTitle>
          <DialogDescription>
            Fully replaces this channel&apos;s configuration. Its recorded
            delivery history stays attached to the same channel id.
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
                        className={`flex flex-col items-center gap-1.5 rounded-lg border p-2.5 text-xs font-medium transition-colors ${
                          selected
                            ? 'border-primary bg-primary/5 text-foreground'
                            : 'border-border text-muted-foreground hover:bg-muted'
                        }`}
                      >
                        {brandIcon ? (
                          <BrandIcon name={brandIcon} className="size-5" />
                        ) : null}
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
            <FieldLabel htmlFor="edit-channel-name">Name</FieldLabel>
            <Input
              id="edit-channel-name"
              placeholder="e.g. Team Slack"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="edit-channel-notify-url">
              {kind === 'email'
                ? 'Notify email address'
                : kind === 'pagerduty'
                  ? 'Integration/Routing Key'
                  : 'Destination (webhook URL or credential)'}
            </FieldLabel>
            <Input
              id="edit-channel-notify-url"
              {...register('notifyUrl', {
                onChange: () => {
                  setVerified(false)
                },
              })}
            />
            <FieldError errors={[formState.errors.notifyUrl]} />
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

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="edit-channel-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="edit-channel-enabled">Enabled</FieldLabel>
          </Field>

          {updateChannel.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{updateChannel.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={updateChannel.isPending}>
              {updateChannel.isPending ? 'Saving...' : 'Save changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
