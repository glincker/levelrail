import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PlusCircleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateDeployNotifyTarget } from '../queries/deployNotify'
import type {
  CreateDeployNotifyTargetRequest,
  DeployNotifyKind,
} from '../types/deployNotify'

const NOTIFY_KIND_OPTIONS: { value: DeployNotifyKind; label: string }[] = [
  { value: 'generic', label: 'Generic webhook' },
  { value: 'slack', label: 'Slack' },
  { value: 'discord', label: 'Discord' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'email', label: 'Email' },
]

// Same per-channel destination placeholder CreateAlertRuleDialog's own
// NOTIFY_DESTINATION_PLACEHOLDER already establishes: every channel but
// email expects a webhook URL, email expects a plain destination
// address (internal/alerting's emailNotifier doc comment).
const NOTIFY_DESTINATION_PLACEHOLDER: Record<DeployNotifyKind, string> = {
  generic: 'https://example.com/webhook',
  slack: 'https://hooks.slack.com/services/...',
  discord: 'https://discord.com/api/webhooks/...',
  telegram: 'https://api.telegram.org/bot<token>/sendMessage?chat_id=...',
  email: 'ops@example.com',
}

const createDeployNotifyTargetSchema = z
  .object({
    notifyUrl: z.string().trim().min(1, 'Destination is required'),
    notifyKind: z.enum(['generic', 'slack', 'discord', 'telegram', 'email']),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    const isValid =
      data.notifyKind === 'email'
        ? z.email().safeParse(data.notifyUrl).success
        : z.url().safeParse(data.notifyUrl).success
    if (!isValid) {
      ctx.addIssue({
        code: 'custom',
        message:
          data.notifyKind === 'email'
            ? 'Must be a valid email address'
            : 'Must be a valid URL',
        path: ['notifyUrl'],
      })
    }
  })

type CreateDeployNotifyTargetForm = z.infer<
  typeof createDeployNotifyTargetSchema
>

const DEFAULT_VALUES: CreateDeployNotifyTargetForm = {
  notifyUrl: '',
  notifyKind: 'generic',
  enabled: true,
}

// Create-via-dialog flow for a new deploy-outcome notify target
// (wave-2 roadmap item #5), following CreateAlertRuleDialog's own shape
// closely (this is the deploy-outcome sibling of that dialog, minus the
// threshold/crashloop condition fields a deploy outcome has no use for):
// a trigger button, a form inside DialogContent, mutate on submit, close
// on success.
export function CreateDeployNotifyTargetDialog({
  appName,
}: {
  appName: string
}) {
  const [open, setOpen] = useState(false)
  const createTarget = useCreateDeployNotifyTarget(appName)
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<CreateDeployNotifyTargetForm>({
      resolver: zodResolver(createDeployNotifyTargetSchema),
      defaultValues: DEFAULT_VALUES,
    })
  const notifyKind = watch('notifyKind')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(DEFAULT_VALUES)
      createTarget.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req: CreateDeployNotifyTargetRequest = {
      notify_url: values.notifyUrl.trim(),
      notify_kind: values.notifyKind,
      enabled: values.enabled,
    }
    createTarget.mutate(req, {
      onSuccess: () => {
        handleOpenChange(false)
        toast.add({
          title: 'Deploy notify target created.',
          type: 'success',
        })
      },
    })
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>Add target</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <PlusCircleIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Add deploy notify target
          </DialogTitle>
          <DialogDescription>
            Notified once per deploy attempt reaching a terminal state
            (succeeded or failed), separate from this app&apos;s threshold and
            crashloop alert rules above.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="deploy-notify-kind">Channel</FieldLabel>
            <Controller
              control={control}
              name="notifyKind"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="deploy-notify-kind" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {NOTIFY_KIND_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="deploy-notify-url">
              {notifyKind === 'email' ? 'Notify email address' : 'Notify URL'}
            </FieldLabel>
            <Input
              id="deploy-notify-url"
              placeholder={NOTIFY_DESTINATION_PLACEHOLDER[notifyKind]}
              {...register('notifyUrl')}
            />
            <FieldError errors={[formState.errors.notifyUrl]} />
          </Field>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="deploy-notify-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="deploy-notify-enabled">Enabled</FieldLabel>
          </Field>

          {createTarget.isError ? (
            <p className="text-sm text-destructive">
              {createTarget.error.message}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createTarget.isPending}>
              {createTarget.isPending ? 'Adding...' : 'Add target'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
